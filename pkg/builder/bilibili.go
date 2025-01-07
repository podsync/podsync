package builder

import (
	"context"

	"strconv"
	"time"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/pkg/errors"
	"github.com/yangtfu/bilibili/v3"

	"github.com/mxpv/podsync/pkg/model"
)

type BilibiliBuilder struct {
	client *bilibili.Client
}

func (b *BilibiliBuilder) queryFeed(feed *model.Feed, info *model.Info) error {
	switch info.LinkType {
	case model.TypeChannel:
		//TODO channel surpport
		return errors.New("bilibili channel not supported")
	case model.TypeUser:
		// query user info
		mid, err := strconv.Atoi(info.ItemID)
		if err != nil {
			return err
		}
		userCardParam := bilibili.GetUserCardParam{Mid: mid, Photo: false}
		userCard, err := b.client.GetUserCard(userCardParam)
		if err != nil {
			return err
		}
		feed.Author = userCard.Card.Name
		feed.CoverArt = userCard.Card.Face
		feed.Title = userCard.Card.Name
		feed.Description = userCard.Card.Sign
		// query video collection
		videoParam := bilibili.GetVideoByKeywordsParam{Mid: mid, Keywords: "", Ps: feed.PageSize}
		videoCollection, err := b.client.GetVideoByKeywords(videoParam)
		if err != nil {
			return err
		}
		feed.PubDate = time.Unix(int64(videoCollection.Archives[0].Pubdate), 0)
		for _, videoInfo := range videoCollection.Archives {
			bvid := videoInfo.Bvid
			desc, err := b.client.GetVideoDesc(bilibili.VideoParam{Bvid: bvid})
			if err == nil {
				e := model.Episode{
					ID:          videoInfo.Bvid,
					Title:       videoInfo.Title,
					Description: desc,
					Duration:    int64(videoInfo.Duration),
					Size:        int64(videoInfo.Duration * 15000), // very rough estimate
					VideoURL:    "https://www.bilibili.com/" + videoInfo.Bvid,
					PubDate:     time.Unix(int64(videoInfo.Pubdate), 0),
					Thumbnail:   videoInfo.Pic,
					Status:      model.EpisodeNew,
				}
				feed.Episodes = append(feed.Episodes, &e)
			}
		}

		return nil
	default:
		return errors.New("unsupported link format")
	}
}

func (b *BilibiliBuilder) Build(_ context.Context, cfg *feed.Config) (*model.Feed, error) {
	info, err := ParseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	_feed := &model.Feed{
		ItemID:          info.ItemID,
		Provider:        info.Provider,
		LinkType:        info.LinkType,
		Format:          cfg.Format,
		Quality:         cfg.Quality,
		CoverArtQuality: cfg.Custom.CoverArtQuality,
		PageSize:        cfg.PageSize,
		PlaylistSort:    cfg.PlaylistSort,
		PrivateFeed:     cfg.PrivateFeed,
		UpdatedAt:       time.Now().UTC(),
		ItemURL:         cfg.URL,
	}

	// Query general information about feed (title, description, lang, etc)
	if err := b.queryFeed(_feed, &info); err != nil {
		return nil, err
	}

	return _feed, nil
}

func NewBilibiliBuilder() (*BilibiliBuilder, error) {
	sc := bilibili.New()

	return &BilibiliBuilder{client: sc}, nil
}
