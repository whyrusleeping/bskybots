package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	cli "github.com/urfave/cli/v2"
	bsky "github.com/whyrusleeping/gosky/api/bsky"
	cliutil "github.com/whyrusleeping/gosky/cmd/gosky/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var hnBotCmd = &cli.Command{
	Name: "hnbot",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "topn",
			Value: 10,
			Usage: "pay attention to the top N posts",
		},
		&cli.DurationFlag{
			Name:  "interval",
			Value: time.Minute,
			Usage: "check for new posts this frequently",
		},
	},
	Action: func(cctx *cli.Context) error {
		atp, err := cliutil.GetATPClient(cctx, false)
		if err != nil {
			return err
		}

		// little db for remembering which posts we've seen across restarts
		db, err := gorm.Open(sqlite.Open("hnbot.db"))
		if err != nil {
			return err
		}

		db.AutoMigrate(&PostRecord{})

		ctx := context.TODO()

		topn := cctx.Int("topn")

		for {
			// refresh the auth every time, technically only need to do this
			// 'every so often', but doing it every time won't break anything
			if err := refreshAuthFile(ctx, atp, cctx.String("auth")); err != nil {
				return fmt.Errorf("auth refresh failed: %w", err)
			}

			fpage, err := fetchFrontPage()
			if err != nil {
				return err
			}

			if len(fpage) > topn {
				fpage = fpage[:topn]
			}

			for _, id := range fpage {
				// check DB for whether or not we've seen this post already
				mp, err := tryFindPost(db, id)
				if err != nil {
					return fmt.Errorf("error checking db for post: %w", err)
				}

				if mp != nil {
					continue
				}

				p, err := getPost(id)
				if err != nil {
					return err
				}

				fmt.Printf("posting: %q\n", p.Title)
				resp, err := atp.RepoCreateRecord(context.TODO(), atp.C.Auth.Did, "app.bsky.feed.post", true, &bsky.FeedPost{
					Text:      p.Title,
					CreatedAt: time.Now().Format("2006-01-02T15:04:05.000Z"),
					Embed: &bsky.FeedPost_Embed{
						EmbedExternal: &bsky.EmbedExternal{
							External: &bsky.EmbedExternal_External{
								Description: "",
								//Thumb         *util.Blob `json:"thumb" cborgen:"thumb"`
								Title: p.Title,
								Uri:   p.Url,
							},
						},
					},
				})
				if err != nil {
					return err
				}

				if err := db.Create(&PostRecord{
					HnId: id,
					Cid:  resp.Cid,
				}).Error; err != nil {
					return err
				}
			}

			time.Sleep(cctx.Duration("interval"))
		}

		return nil
	},
}

const hnURL = "https://hacker-news.firebaseio.com"

func fetchFrontPage() ([]int64, error) {
	resp, err := http.Get(hnURL + "/v0/topstories.json")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("non-200 response from server: %s (%d)", resp.Status, resp.StatusCode)
	}

	var out []int64
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return out, nil
}

type HnPost struct {
	ID          int64
	By          string
	Descendants int64
	Kids        []int64
	Score       int64
	Time        int64
	Title       string
	Type        string
	Url         string
}

func getPost(id int64) (*HnPost, error) {
	resp, err := http.Get(fmt.Sprintf("%s/v0/item/%d.json", hnURL, id))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("non-200 response from server: %s (%d)", resp.Status, resp.StatusCode)
	}

	var out HnPost
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

type PostRecord struct {
	gorm.Model

	HnId int64
	Cid  string
}

func tryFindPost(db *gorm.DB, id int64) (*PostRecord, error) {
	var maybePost PostRecord
	if err := db.Find(&maybePost, "hn_id = ?", id).Error; err != nil {
		return nil, err
	}

	if maybePost.ID != 0 {
		return &maybePost, nil
	}

	return nil, nil
}
