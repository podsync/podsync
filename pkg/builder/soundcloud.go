package builder

import (
	"context"
	"strconv"
	"time"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/pkg/errors"
	soundcloudapi "github.com/zackradisic/soundcloud-api"

	"github.com/mxpv/podsync/pkg/model"
)

type SoundCloudBuilder struct {
	client *soundcloudapi.API
}

func (s *SoundCloudBuilder) Build(_ context.Context, cfg *feed.Config) (*model.Feed, error) {
	info, err := ParseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	_feed := &model.Feed{
		ItemID:    info.ItemID,
		Provider:  info.Provider,
		LinkType:  info.LinkType,
		Format:    cfg.Format,
		Quality:   cfg.Quality,
		PageSize:  cfg.PageSize,
		UpdatedAt: time.Now().UTC(),
	}

	if info.LinkType == model.TypePlaylist {
		if soundcloudapi.IsPlaylistURL(cfg.URL) {
			scplaylist, err := s.client.GetPlaylistInfo(cfg.URL)
			if err != nil {
				return nil, err
			}

			_feed.Title = scplaylist.Title
			_feed.Description = scplaylist.Description
			_feed.ItemURL = cfg.URL

			date, err := time.Parse(time.RFC3339, scplaylist.CreatedAt)
			if err == nil {
				_feed.PubDate = date
			}
			_feed.Author = scplaylist.User.Username
			_feed.CoverArt = scplaylist.ArtworkURL

			var added = 0
			for _, track := range scplaylist.Tracks {
				pubDate, _ := time.Parse(time.RFC3339, track.CreatedAt)
				var (
					videoID   = strconv.FormatInt(track.ID, 10)
					duration  = track.DurationMS / 1000
					mediaURL  = track.PermalinkURL
					trackSize = track.DurationMS * 15 // very rough estimate
				)

				_feed.Episodes = append(_feed.Episodes, &model.Episode{
					ID:          videoID,
					Title:       track.Title,
					Description: track.Description,
					Duration:    duration,
					Size:        trackSize,
					VideoURL:    mediaURL,
					PubDate:     pubDate,
					Thumbnail:   track.ArtworkURL,
					Status:      model.EpisodeNew,
				})

				added++

				if added >= _feed.PageSize {
					return _feed, nil
				}
			}

			return _feed, nil
		}
	}

	return nil, errors.New(("unsupported soundcloud feed type"))
}

func NewSoundcloudBuilder() (*SoundCloudBuilder, error) {
	sc, err := soundcloudapi.New(soundcloudapi.APIOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create soundcloud client")
	}

	return &SoundCloudBuilder{client: sc}, nil
}
