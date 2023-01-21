package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	atproto "github.com/bluesky-social/indigo/api/atproto"
	bsky "github.com/bluesky-social/indigo/api/bsky"
	cliutil "github.com/bluesky-social/indigo/cmd/gosky/util"
	cli "github.com/urfave/cli/v2"
	"github.com/whyrusleeping/openai"
)

var gptCmd = &cli.Command{
	Name: "gpt",
	Subcommands: []*cli.Command{
		gptRespondCmd,
	},
}

type ParamsFile struct {
	Prompt string
	Model  string
	Auth   string
	Org    string
}

func loadBotParams(f string) (*ParamsFile, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}

	var params ParamsFile
	if err := json.Unmarshal(b, &params); err != nil {
		return nil, err
	}

	return &params, nil
}

var gptRespondCmd = &cli.Command{
	Name: "respond",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "params",
			Usage: "params file for the bot",
		},
	},
	Action: func(cctx *cli.Context) error {
		post := cctx.Args().First()

		atp, err := cliutil.GetATPClient(cctx, false)
		if err != nil {
			return err
		}

		ctx := context.TODO()

		bot, err := loadBotParams(cctx.String("params"))
		if err != nil {
			return err
		}

		if err := refreshAuthFile(ctx, atp, cctx.String("auth")); err != nil {
			return fmt.Errorf("auth refresh failed: %w", err)
		}

		parts := strings.Split(post, "/")

		rec, err := atproto.RepoGetRecord(ctx, atp.C, "", parts[len(parts)-2], parts[len(parts)-1], parts[len(parts)-3])
		if err != nil {
			return err
		}

		fp := rec.Value.Val.(*bsky.FeedPost)

		gptc := openai.Client{
			Auth: bot.Auth,
			Org:  bot.Org,
		}

		resp, err := gptc.Completion(ctx, &openai.CompletionRequest{
			Model:       bot.Model,
			Prompt:      fmt.Sprintf(bot.Prompt, fp.Text),
			Temperature: 0.5,
			MaxTokens:   40,
		})

		for _, c := range resp.Choices {
			fmt.Println(c.Text)
		}

		pref := &atproto.RepoStrongRef{
			Cid: *rec.Cid,
			Uri: post,
		}
		root := pref
		if fp.Reply != nil {
			root = fp.Reply.Root
		}

		recresp, err := atp.RepoCreateRecord(ctx, atp.C.Auth.Did, "app.bsky.feed.post", true, &bsky.FeedPost{
			CreatedAt: time.Now().Format("2006-01-02T15:04:05.000Z"),
			Reply: &bsky.FeedPost_ReplyRef{
				Parent: pref,
				Root:   root,
			},
			Text: resp.Choices[0].Text,
		})
		if err != nil {
			return err
		}

		fmt.Println(recresp)

		return nil
	},
}
