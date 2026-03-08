package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"

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

	inputTools := []tools.Tool{&RollDiceTool{}}

	runnable, err := prebuilt.CreateAgentMap(llm, inputTools, 10)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(
				llms.ChatMessageTypeHuman,
				"Roll a dice for 3 times and tell me the result.",
			),
		},
	}

	resp, err := runnable.Invoke(ctx, initialState)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp)
}

type RollDiceTool struct{}

func (t *RollDiceTool) Name() string {
	return "roll_dice"
}

func (t *RollDiceTool) Description() string {
	return "Roll a 6-sided dice and return the result."
}

func (t *RollDiceTool) Call(ctx context.Context, input string) (string, error) {
	return strconv.Itoa(rand.Intn(6) + 1), nil
}
