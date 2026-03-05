package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/prebuilt"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

func main() {
	godotenv.Load()

	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	runnable, err := prebuilt.CreateAgentMap(llm, []tools.Tool{&GetCurrentDateTimeTool{}}, 10)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeHuman, "What is the current date and time?"),
		},
	}

	resp, err := runnable.Invoke(ctx, initialState)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp)
}

type GetCurrentDateTimeTool struct{}

func (t *GetCurrentDateTimeTool) Name() string {
	return "get_current_date_time"
}

func (t *GetCurrentDateTimeTool) Description() string {
	return "Get the current date and time"
}

func (t *GetCurrentDateTimeTool) Call(ctx context.Context, input string) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}
