package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/bluesky-social/indigo/api"
	cli "github.com/urfave/cli/v2"
)

func main() {

	app := cli.NewApp()

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "pds",
			Value: "https://pds.staging.bsky.dev",
		},
		&cli.StringFlag{
			Name: "auth",
		},
	}
	app.Commands = []*cli.Command{
		hnBotCmd,
		gptCmd,
	}

	app.RunAndExitOnError()
}

func refreshAuthFile(ctx context.Context, atpc *api.ATProto, fname string) error {
	a := atpc.C.Auth
	a.AccessJwt = a.RefreshJwt

	nauth, err := atpc.SessionRefresh(ctx)
	if err != nil {
		return err
	}

	b, err := json.Marshal(nauth)
	if err != nil {
		return err
	}

	if err := os.WriteFile(fname, b, 0600); err != nil {
		return err
	}

	atpc.C.Auth = nauth

	return nil

}
