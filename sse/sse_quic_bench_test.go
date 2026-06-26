package sse

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/xDarkicex/nanite"
	"github.com/xDarkicex/nanite/quic"
)

func generateTestCerts() error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Bench Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create("test_cert.pem")
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.Create("test_key.pem")
	if err != nil {
		return err
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	keyOut.Close()

	return nil
}

func BenchmarkSSEOverQUIC(b *testing.B) {
	if err := generateTestCerts(); err != nil {
		b.Fatal(err)
	}
	defer os.Remove("test_cert.pem")
	defer os.Remove("test_key.pem")

	r := nanite.New()
	
	// Server route
	Register(r, "/stream", func(conn *Connection, c *nanite.Context) {
		payloadEvent := StringToBytes("bench")
		payloadData := StringToBytes("test payload data for benchmark")
		for i := 0; i < b.N; i++ {
			_ = conn.Send(payloadEvent, payloadData)
		}
	})

	qs := quic.New(r, quic.Config{
		Addr:     "127.0.0.1:44383",
		CertFile: "test_cert.pem",
		KeyFile:  "test_key.pem",
	})
	
	go func() {
		_ = qs.StartHTTP3()
	}()
	defer qs.ShutdownGraceful(time.Second)

	time.Sleep(200 * time.Millisecond) // Let server start

	tr := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	defer tr.Close()

	client := &http.Client{Transport: tr}
	
	b.ResetTimer()
	b.ReportAllocs()

	req, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1:44383/stream", nil)
	resp, err := client.Do(req)
	if err != nil {
		b.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	for {
		_, err := resp.Body.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSSEOverQUIC_Concurrent(b *testing.B) {
	if err := generateTestCerts(); err != nil {
		b.Fatal(err)
	}
	defer os.Remove("test_cert.pem")
	defer os.Remove("test_key.pem")

	r := nanite.New()

	// Server route
	Register(r, "/stream_concurrent", func(conn *Connection, c *nanite.Context) {
		payloadEvent := StringToBytes("bench")
		payloadData := StringToBytes("test payload data for benchmark")
		for {
			// Pump as fast as possible. Exit when client closes connection.
			err := conn.Send(payloadEvent, payloadData)
			if err != nil {
				return
			}
		}
	})

	qs := quic.New(r, quic.Config{
		Addr:     "127.0.0.1:44384",
		CertFile: "test_cert.pem",
		KeyFile:  "test_key.pem",
	})

	go func() {
		_ = qs.StartHTTP3()
	}()
	defer qs.ShutdownGraceful(time.Second)

	time.Sleep(200 * time.Millisecond) // Let server start

	tr := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	defer tr.Close()

	client := &http.Client{Transport: tr}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		req, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1:44384/stream_concurrent", nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Error(err)
			return
		}
		defer resp.Body.Close()

		buf := make([]byte, 8192)
		for pb.Next() {
			_, err := resp.Body.Read(buf)
			if err != nil && err != io.EOF {
				b.Error(err)
				break
			}
		}
	})
}
