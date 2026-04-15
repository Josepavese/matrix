// Package zedacp provides a client-side Go implementation of the Zed Agent Client Protocol (ACP).
//
// The package is intentionally shaped like an SDK:
//   - schema models for ACP requests, responses, notifications, and tool calls
//   - a client-side JSON-RPC connection with typed ACP methods
//   - transport implementations for stdio, websocket, and unix sockets
//   - narrow interfaces for request handling and session observation
//
// Matrix uses this package through an adapter layer, but the package is designed
// to be separable into a standalone repository in the future.
package zedacp
