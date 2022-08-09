package builder

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"
	"github.com/nicklaw5/helix"
	"github.com/pkg/errors"
)

type TwitchBuilder struct {
	client *helix.Client
}

func (t *TwitchBuilder) Build(_ctx context.Context, cfg *feed.Config) (*model.Feed, error) {
	info, err := ParseURL(cfg.URL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}

	feed := &model.Feed{
		ItemID:    info.ItemID,
		Provider:  info.Provider,
		LinkType:  info.LinkType,
		Format:    cfg.Format,
		Quality:   cfg.Quality,
		PageSize:  cfg.PageSize,
		UpdatedAt: time.Now().UTC(),
	}

	if info.LinkType == model.TypeUser {

		users, err := t.client.GetUsers(&helix.UsersParams{
			Logins: []string{info.ItemID},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get user: %s", info.ItemID)
		}
		user := users.Data.Users[0]

		feed.Title = user.DisplayName
		feed.Author = user.DisplayName
		feed.Description = user.Description
		feed.ItemURL = fmt.Sprintf("https://www.twitch.tv/%s", user.Login)
		feed.CoverArt = user.ProfileImageURL
		feed.PubDate = user.CreatedAt.Time

		isStreaming := false
		streamID := ""
		streams, err := t.client.GetStreams(&helix.StreamsParams{
			UserIDs: []string{user.ID},
		})
		if len(streams.Data.Streams) > 0 {
			isStreaming = true
			streamID = streams.Data.Streams[0].ID
		}

		videos, err := t.client.GetVideos(&helix.VideosParams{
			UserID: user.ID,
			Period: "all",
			Type:   "archive",
			Sort:   "time",
			First:  10,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get videos for user: %s", info.ItemID)
		}

		var added = 0
		for _, video := range videos.Data.Videos {

			// Do not add the video of an ongoing stream because it will be incomplete
			if !isStreaming || video.StreamID != streamID {

				date, err := time.Parse(time.RFC3339, video.PublishedAt)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot parse PublishedAt time: %s", video.PublishedAt)
				}

				replacer := strings.NewReplacer("%{width}", "300", "%{height}", "300")
				thumbnailUrl := replacer.Replace(video.ThumbnailURL)

				duration, err := time.ParseDuration(video.Duration)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot parse duration: %s", video.Duration)
				}
				durationSeconds := int64(duration.Seconds())

				feed.Episodes = append(feed.Episodes, &model.Episode{
					ID:          video.ID,
					Title:       fmt.Sprintf("%s (%s)", video.Title, date),
					Description: video.Description,
					Thumbnail:   thumbnailUrl,
					Duration:    durationSeconds,
					Size:        durationSeconds * 33013, // Very rough estimate
					VideoURL:    video.URL,
					PubDate:     date,
					Status:      model.EpisodeNew,
				})

				added++
				if added >= feed.PageSize {
					return feed, nil
				}
			}

		}

		return feed, nil

	}

	return nil, errors.New("unsupported feed type")
}

func NewTwitchBuilder(clientIDSecret string) (*TwitchBuilder, error) {
	parts := strings.Split(clientIDSecret, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid twitch key, need to be \"CLIENT_ID:CLIENT_SECRET\"")
	}

	clientID := parts[0]
	clientSecret := parts[1]

	client, err := helix.NewClient(&helix.Options{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create twitch client")
	}

	token, err := client.RequestAppAccessToken([]string{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to request twitch app token")
	}

	// Set the access token on the client
	client.SetAppAccessToken(token.Data.AccessToken)

	return &TwitchBuilder{client: client}, nil
}
