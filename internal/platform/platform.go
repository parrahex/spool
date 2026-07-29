package platform

import "context"

type File struct {
	ID   string
	Name string
}

type Message struct {
	Text    string
	Files   []File
	Channel string
	Thread  string
	Sender  string
}

type HandlerFunc func(ctx context.Context, msg Message)

type Platform interface {
	Listen(ctx context.Context, handler HandlerFunc) error
	Reply(ctx context.Context, msg Message, text string) error
	Download(ctx context.Context, f File, dest string) error
}
