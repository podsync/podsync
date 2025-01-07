package builder

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/pkg/errors"
	"github.com/silentsokolov/go-vimeo/vimeo"
	"golang.org/x/oauth2"

	"github.com/mxpv/podsync/pkg/model"
)

const (
	vimeoDefaultPageSize = 50
)

type VimeoBuilder struct {
	client *vimeo.Client
}

func (v *VimeoBuilder) selectImage(p *vimeo.Pictures, q model.Quality) string {
	if p == nil || len(p.Sizes) == 0 {
		return ""
	}

	if q == model.QualityLow {
		return p.Sizes[0].Link
	}

	return p.Sizes[len(p.Sizes)-1].Link
}

func (v *VimeoBuilder) queryChannel(feed *model.Feed) error {
	channelID := feed.ItemID

	ch, resp, err := v.client.Channels.Get(channelID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return model.ErrNotFound
		}

		return errors.Wrapf(err, "failed to query channel with id %q", channelID)
	}

	feed.Title = ch.Name
	feed.ItemURL = ch.Link
	feed.Description = ch.Description
	feed.CoverArt = v.selectImage(ch.Pictures, feed.Quality)
	feed.Author = ch.User.Name
	feed.PubDate = ch.CreatedTime
	feed.UpdatedAt = time.Now().UTC()

	return nil
}

func (v *VimeoBuilder) queryGroup(feed *model.Feed) error {
	groupID := feed.ItemID

	gr, resp, err := v.client.Groups.Get(groupID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return model.ErrNotFound
		}

		return errors.Wrapf(err, "failed to query group with id %q", groupID)
	}

	feed.Title = gr.Name
	feed.ItemURL = gr.Link
	feed.Description = gr.Description
	feed.CoverArt = v.selectImage(gr.Pictures, feed.Quality)
	feed.Author = gr.User.Name
	feed.PubDate = gr.CreatedTime
	feed.UpdatedAt = time.Now().UTC()

	return nil
}

func (v *VimeoBuilder) queryUser(feed *model.Feed) error {
	userID := feed.ItemID

	user, resp, err := v.client.Users.Get(userID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return model.ErrNotFound
		}

		return errors.Wrapf(err, "failed to query user with id %q", userID)
	}

	feed.Title = user.Name
	feed.ItemURL = user.Link
	feed.Description = user.Bio
	feed.CoverArt = v.selectImage(user.Pictures, feed.Quality)
	feed.Author = user.Name
	feed.PubDate = user.CreatedTime
	feed.UpdatedAt = time.Now().UTC()

	return nil
}

func (v *VimeoBuilder) getVideoSize(video *vimeo.Video) int64 {
	// Very approximate video file size
	return int64(float64(video.Duration*video.Width*video.Height) * 0.38848958333)
}

type getVideosFunc func(string, ...vimeo.CallOption) ([]*vimeo.Video, *vimeo.Response, error)

func (v *VimeoBuilder) queryVideos(getVideos getVideosFunc, feed *model.Feed) error {
	var (
		page  = 1
		added = 0
	)

	for {
		videos, response, err := getVideos(feed.ItemID, vimeo.OptPage(page), vimeo.OptPerPage(vimeoDefaultPageSize))
		if err != nil {
			if response != nil {
				return errors.Wrapf(err, "failed to query videos (error %d %s)", response.StatusCode, response.Status)
			}

			return err
		}

		for _, video := range videos {
			var (
				videoID  = strconv.Itoa(video.GetID())
				videoURL = video.Link
				duration = int64(video.Duration)
				size     = v.getVideoSize(video)
				image    = v.selectImage(video.Pictures, feed.Quality)
			)

			feed.Episodes = append(feed.Episodes, &model.Episode{
				ID:          videoID,
				Title:       video.Name,
				Description: video.Description,
				Duration:    duration,
				Size:        size,
				PubDate:     video.CreatedTime,
				Thumbnail:   image,
				VideoURL:    videoURL,
				Status:      model.EpisodeNew,
			})

			added++
		}

		if added >= feed.PageSize || response.NextPage == "" {
			return nil
		}

		page++
	}
}

func (v *VimeoBuilder) Build(_ context.Context, cfg *feed.Config) (*model.Feed, error) {
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

	if info.LinkType == model.TypeChannel {
		if err := v.queryChannel(_feed); err != nil {
			return nil, err
		}

		if err := v.queryVideos(v.client.Channels.ListVideo, _feed); err != nil {
			return nil, err
		}

		return _feed, nil
	}

	if info.LinkType == model.TypeGroup {
		if err := v.queryGroup(_feed); err != nil {
			return nil, err
		}

		if err := v.queryVideos(v.client.Groups.ListVideo, _feed); err != nil {
			return nil, err
		}

		return _feed, nil
	}

	if info.LinkType == model.TypeUser {
		if err := v.queryUser(_feed); err != nil {
			return nil, err
		}

		if err := v.queryVideos(v.client.Users.ListVideo, _feed); err != nil {
			return nil, err
		}

		return _feed, nil
	}

	return nil, errors.New("unsupported feed type")
}

func NewVimeoBuilder(ctx context.Context, token string) (*VimeoBuilder, error) {
	if token == "" {
		return nil, errors.New("empty Vimeo access token")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	client := vimeo.NewClient(tc, nil)
	return &VimeoBuilder{client}, nil
}
