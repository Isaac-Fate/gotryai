package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"

	"github.com/cloudwego/eino-examples/quickstart/chatwitheino/chatmodel"
	"github.com/cloudwego/eino-examples/quickstart/chatwitheino/msgops"
)

func main() {
	godotenv.Load(".env")

	var instruction string
	flag.StringVar(&instruction, "instruction", "You are a helpful assistant.", "")
	flag.Parse()

	query := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if query == "" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: go run ./examples/eino/main.go -- \"your question\"")
		os.Exit(2)
	}

	ctx := context.Background()
	switch msgops.KindFromEnv() {
	case msgops.KindAgentic:
		runTyped[*schema.AgenticMessage](ctx, instruction, query)
	default:
		runTyped[*schema.Message](ctx, instruction, query)
	}
}

func runTyped[M adk.MessageType](ctx context.Context, instruction, query string) {
	cm, err := chatmodel.NewModel[M](ctx)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	messages := []M{
		msgops.NewSystem[M](instruction),
		msgops.NewUser[M](query),
	}

	_, _ = fmt.Fprint(os.Stdout, "[assistant] ")
	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer stream.Close()

	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !msgops.IsNil(frame) {
			_, _ = fmt.Fprint(os.Stdout, msgops.AssistantDeltaText(frame))
		}
	}
	_, _ = fmt.Fprintln(os.Stdout)
}
