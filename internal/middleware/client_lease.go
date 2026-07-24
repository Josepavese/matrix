package middleware

// ConversationClientLeaser lets a router hold a client alive while one routed
// operation is using it. The release result reports whether deferred Close
// completed so process-reap evidence can be recorded at the real close boundary.
type ConversationClientLeaser interface {
	AcquireConversationClientLease() (release func() (closed bool, err error), err error)
}
