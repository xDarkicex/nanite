# Abort Behavior Documentation Updates

## Summary

Updated NANITE_USAGE.md to reflect the corrected abort behavior implemented in the codebase.

## Key Documentation Changes

### 1. Post-Handler Phase Behavior

**Before:**
```
3. **Post-handler phase** (runs after handler completes):
   - If error is stored AND no response written: UseError middleware runs
   - If response already written: nothing runs (response is final)
```

**After:**
```
3. **Post-handler phase** (runs after handler completes):
   - If request was aborted AND no response written: 404 Not Found is returned
   - If error is stored AND no response written: UseError middleware runs
   - If response already written: nothing runs (response is final)
```

### 2. Key Points About Abort

**Before:**
- `c.Abort()` simply stops the middleware chain. It's the middleware's responsibility to write a response before aborting.
- If middleware aborts without writing a response, the request ends with no response body (middleware author's responsibility).

**After:**
- `c.Abort()` stops the middleware chain. If no response is written before aborting, a 404 is automatically returned.
- If middleware aborts after writing a response, that response is sent (no 404 override).
- Calling `next()` after `Abort()` is a no-op - the chain will not continue.

### 3. New Example Added

Added a new example showing abort without response:

```go
// Option 2: Abort without response (returns 404)
authMiddleware := func(c *nanite.Context, next func()) {
    token := c.Request.Header.Get("Authorization")
    if token == "" {
        c.Abort()  // Stop chain, 404 will be returned
        return
    }
    next()
}
```

## Behavior Summary

When `c.Abort()` is called:

1. **Middleware chain stops immediately**
   - Remaining middleware in the chain will not execute
   - The handler will not execute
   - Calling `next()` after `Abort()` has no effect

2. **Response handling**
   - If a response was written before abort: That response is sent
   - If no response was written: 404 Not Found is automatically returned
   - Error middleware and panic recovery can still write responses even when aborted

3. **Use cases**
   - **With response**: Write custom error response, then abort (e.g., 401 Unauthorized)
   - **Without response**: Abort and let framework return 404 (e.g., silent rejection)
   - **With error**: Use `c.Error()` to store error and abort, let error middleware handle response

## Testing

Both abort-related tests now pass:
- `TestContextAborted` - Tests abort with explicit `next()` call
- `TestMiddlewareAbortPropagation` - Tests abort without `next()` call

All existing tests continue to pass, confirming backward compatibility.
