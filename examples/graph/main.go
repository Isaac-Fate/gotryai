package main

import (
	"context"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
)

func main() {
	godotenv.Load()

	// llm, err := openai.New(
	// 	openai.WithBaseURL("https://api.deepseek.com"),
	// 	openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
	// 	openai.WithModel("deepseek-chat"),
	// )
	// if err != nil {
	// 	panic(err)
	// }

	// inputTools := []tools.Tool{}

	g := graph.NewStreamingStateGraph[MyState]()

	ctx := context.Background()

	initialState := MyState{}

	runnable, err := g.CompileListenable()
	if err != nil {
		panic(err)
	}

	runnable.Stream(ctx, initialState)

}

type MyState map[string]any
