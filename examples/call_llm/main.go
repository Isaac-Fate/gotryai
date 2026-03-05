package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms/openai"
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

	ctx := context.Background()

	resp, err := llm.Call(ctx, "Who are you?")
	if err != nil {
		panic(err)
	}

	fmt.Println(resp)
}
