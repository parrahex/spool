package platform

import "context"

// File identifies an attachment uploaded with a platform message
type File struct {
	ID   string
	Name string
}

// Message is a normalized representation of an incoming platform message;
// bot code never touches platform-specific types
type Message struct {
	Text    string
	Files   []File
	Channel string
	Thread  string
	Sender  string
}

// HandlerFunc processes one incoming message; implementations must not
// assume it runs on the platform's event goroutine
type HandlerFunc func(ctx context.Context, msg Message)

// Platform abstracts the messaging backend so bot logic stays testable
// and independent of Slack specifics
type Platform interface {
	Listen(ctx context.Context, handler HandlerFunc) error
	Reply(ctx context.Context, msg Message, text string) error
	Download(ctx context.Context, f File, dest string) error
}
