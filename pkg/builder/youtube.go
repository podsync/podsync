package builder

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BrianHicks/finch/duration"
	"github.com/mxpv/podsync/pkg/feed"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/youtube/v3"

	"github.com/mxpv/podsync/pkg/model"
)

const (
	maxYoutubeResults       = 50
	hdBytesPerSecond        = 350000
	ldBytesPerSecond        = 100000
	lowAudioBytesPerSecond  = 48000 / 8
	highAudioBytesPerSecond = 128000 / 8
)

type apiKey string

func (key apiKey) Get() (string, string) {
	return "key", string(key)
}

type YouTubeBuilder struct {
	client *youtube.Service
	key    apiKey
}

// Cost: 5 units (call method: 1, snippet: 2, contentDetails: 2)
// See https://developers.google.com/youtube/v3/docs/channels/list#part
func (yt *YouTubeBuilder) listChannels(ctx context.Context, linkType model.Type, id string, parts string) (*youtube.Channel, error) {
	req := yt.client.Channels.List(parts)

	switch linkType {
	case model.TypeChannel:
		req = req.Id(id)
	case model.TypeUser:
		req = req.ForUsername(id)
	default:
		return nil, errors.New("unsupported link type")
	}

	resp, err := req.Context(ctx).Do(yt.key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query channel")
	}

	if len(resp.Items) == 0 {
		return nil, model.ErrNotFound
	}

	item := resp.Items[0]
	return item, nil
}

// Cost: 3 units (call method: 1, snippet: 2)
// See https://developers.google.com/youtube/v3/docs/playlists/list#part
func (yt *YouTubeBuilder) listPlaylists(ctx context.Context, id, channelID string, parts string) (*youtube.Playlist, error) {
	req := yt.client.Playlists.List(parts)

	if id != "" {
		req = req.Id(id)
	} else {
		req = req.ChannelId(channelID)
	}

	resp, err := req.Context(ctx).Do(yt.key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query playlist")
	}

	if len(resp.Items) == 0 {
		return nil, model.ErrNotFound
	}

	item := resp.Items[0]
	return item, nil
}

// Cost: 3 units (call: 1, snippet: 2)
// See https://developers.google.com/youtube/v3/docs/playlistItems/list#part
func (yt *YouTubeBuilder) listPlaylistItems(ctx context.Context, feed *model.Feed, pageToken string) ([]*youtube.PlaylistItem, string, error) {
	count := maxYoutubeResults
	if count > feed.PageSize {
		// If we need less than 50
		count = feed.PageSize
	}

	req := yt.client.PlaylistItems.List("id,snippet").MaxResults(int64(count)).PlaylistId(feed.ItemID)
	if pageToken != "" {
		req = req.PageToken(pageToken)
	}

	resp, err := req.Context(ctx).Do(yt.key)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to query playlist items")
	}

	return resp.Items, resp.NextPageToken, nil
}

func (yt *YouTubeBuilder) parseDate(s string) (time.Time, error) {
	date, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to parse date: %s", s)
	}

	return date, nil
}

func (yt *YouTubeBuilder) selectThumbnail(snippet *youtube.ThumbnailDetails, quality model.Quality, videoID string) string {
	if snippet == nil {
		if videoID != "" {
			return fmt.Sprintf("https://img.youtube.com/vi/%s/default.jpg", videoID)
		}

		// TODO: use Podsync's preview image if unable to retrieve from YouTube
		return ""
	}

	// Use high resolution thumbnails for high quality mode
	// https://github.com/mxpv/Podsync/issues/14
	if quality == model.QualityHigh {
		if snippet.Maxres != nil {
			return snippet.Maxres.Url
		}

		if snippet.High != nil {
			return snippet.High.Url
		}

		if snippet.Medium != nil {
			return snippet.Medium.Url
		}
	}

	return snippet.Default.Url
}

func (yt *YouTubeBuilder) GetVideoCount(ctx context.Context, info *model.Info) (uint64, error) {
	switch info.LinkType {
	case model.TypeChannel, model.TypeUser:
		// Cost: 3 units
		if channel, err := yt.listChannels(ctx, info.LinkType, info.ItemID, "id,statistics"); err != nil {
			return 0, err
		} else { // nolint:golint
			return channel.Statistics.VideoCount, nil
		}

	case model.TypePlaylist:
		// Cost: 3 units
		if playlist, err := yt.listPlaylists(ctx, info.ItemID, "", "id,contentDetails"); err != nil {
			return 0, err
		} else { // nolint:golint
			return uint64(playlist.ContentDetails.ItemCount), nil
		}

	default:
		return 0, errors.New("unsupported link format")
	}
}

func (yt *YouTubeBuilder) queryFeed(ctx context.Context, feed *model.Feed, info *model.Info) error {
	var (
		thumbnails *youtube.ThumbnailDetails
	)

	switch info.LinkType {
	case model.TypeChannel, model.TypeUser:
		// Cost: 5 units for channel or user
		channel, err := yt.listChannels(ctx, info.LinkType, info.ItemID, "id,snippet,contentDetails")
		if err != nil {
			return err
		}

		feed.Title = channel.Snippet.Title
		feed.Description = channel.Snippet.Description

		if channel.Kind == "youtube#channel" {
			feed.ItemURL = fmt.Sprintf("https://youtube.com/channel/%s", channel.Id)
			feed.Author = "<notfound>"
		} else {
			feed.ItemURL = fmt.Sprintf("https://youtube.com/user/%s", channel.Snippet.CustomUrl)
			feed.Author = channel.Snippet.CustomUrl
		}

		feed.ItemID = channel.ContentDetails.RelatedPlaylists.Uploads

		if date, err := yt.parseDate(channel.Snippet.PublishedAt); err != nil {
			return err
		} else { // nolint:golint
			feed.PubDate = date
		}

		thumbnails = channel.Snippet.Thumbnails

	case model.TypePlaylist:
		// Cost: 3 units for playlist
		playlist, err := yt.listPlaylists(ctx, info.ItemID, "", "id,snippet")
		if err != nil {
			return err
		}

		feed.Title = fmt.Sprintf("%s: %s", playlist.Snippet.ChannelTitle, playlist.Snippet.Title)
		feed.Description = playlist.Snippet.Description

		feed.ItemURL = fmt.Sprintf("https://youtube.com/playlist?list=%s", playlist.Id)
		feed.ItemID = playlist.Id

		feed.Author = "<notfound>"

		if date, err := yt.parseDate(playlist.Snippet.PublishedAt); err != nil {
			return err
		} else { // nolint:golint
			feed.PubDate = date
		}

		thumbnails = playlist.Snippet.Thumbnails

	default:
		return errors.New("unsupported link format")
	}

	if feed.Description == "" {
		feed.Description = fmt.Sprintf("%s (%s)", feed.Title, feed.PubDate)
	}

	feed.CoverArt = yt.selectThumbnail(thumbnails, feed.CoverArtQuality, "")

	return nil
}

// Video size information requires 1 additional call for each video (1 feed = 50 videos = 50 calls),
// which is too expensive, so get approximated size depending on duration and definition params
func (yt *YouTubeBuilder) getSize(duration int64, feed *model.Feed) int64 {
	if feed.Format == model.FormatAudio {
		if feed.Quality == model.QualityHigh {
			return highAudioBytesPerSecond * duration
		}

		return lowAudioBytesPerSecond * duration
	}

	// Video format

	if feed.Quality == model.QualityHigh {
		return duration * hdBytesPerSecond
	}

	return duration * ldBytesPerSecond
}

// Cost: 5 units (call: 1, snippet: 2, contentDetails: 2)
// See https://developers.google.com/youtube/v3/docs/videos/list#part
func (yt *YouTubeBuilder) queryVideoDescriptions(ctx context.Context, playlist map[string]*youtube.PlaylistItemSnippet, feed *model.Feed) error {
	// Make the list of video ids
	ids := make([]string, 0, len(playlist))
	for _, s := range playlist {
		ids = append(ids, s.ResourceId.VideoId)
	}

	// Init a list that will contains the aggregated strings of videos IDs (capped at 50 IDs per API Calls)
	idsList := make([]string, 0, 1)

	// Chunk the list of IDs by slices limited to maxYoutubeResults
	for i := 0; i < len(ids); i += maxYoutubeResults {
		end := i + maxYoutubeResults
		if end > len(ids) {
			end = len(ids)
		}
		// Save each slice as comma-delimited string
		idsList = append(idsList, strings.Join(ids[i:end], ","))
	}

	// Show how many API calls will be required
	log.Debugf("Expected to make %d API calls to get the descriptions for %d episode(s).", len(idsList), len(ids))

	// Loop in each slices of 50 (or less) IDs and query their description
	for _, idsI := range idsList {
		req, err := yt.client.Videos.List("id,snippet,contentDetails").Id(idsI).Context(ctx).Do(yt.key)
		if err != nil {
			return errors.Wrap(err, "failed to query video descriptions")
		}

		for _, video := range req.Items {
			var (
				snippet  = video.Snippet
				videoID  = video.Id
				videoURL = fmt.Sprintf("https://youtube.com/watch?v=%s", video.Id)
				image    = yt.selectThumbnail(snippet.Thumbnails, feed.Quality, videoID)
			)

			// Parse date added to playlist / publication date
			dateStr := ""
			playlistItem, ok := playlist[video.Id]
			if ok {
				dateStr = playlistItem.PublishedAt
			} else {
				dateStr = snippet.PublishedAt
			}

			pubDate, err := yt.parseDate(dateStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse video publish date: %s", dateStr)
			}

			// Sometimes YouTube retrun empty content defailt, use arbitrary one
			var seconds int64 = 1
			if video.ContentDetails.Duration != "" {
				// Parse duration
				d, err := duration.FromString(video.ContentDetails.Duration)
				if err != nil {
					return errors.Wrapf(err, "failed to parse duration %s", video.ContentDetails.Duration)
				}

				seconds = int64(d.ToDuration().Seconds())
			} else {
				continue
			}

			var (
				order = strconv.FormatInt(playlistItem.Position, 10)
				size  = yt.getSize(seconds, feed)
			)

			feed.Episodes = append(feed.Episodes, &model.Episode{
				ID:          video.Id,
				Title:       snippet.Title,
				Description: snippet.Description,
				Thumbnail:   image,
				Duration:    seconds,
				Size:        size,
				VideoURL:    videoURL,
				PubDate:     pubDate,
				Order:       order,
				Status:      model.EpisodeNew,
			})
		}
	}

	return nil
}

// Cost:
// ASC mode = (3 units + 5 units) * X pages = 8 units per page
// DESC mode = 3 units * (number of pages in the entire playlist) + 5 units
func (yt *YouTubeBuilder) queryItems(ctx context.Context, feed *model.Feed) error {
	var (
		token       string
		count       int
		allSnippets []*youtube.PlaylistItemSnippet
	)

	for {
		items, pageToken, err := yt.listPlaylistItems(ctx, feed, token)
		if err != nil {
			return err
		}

		token = pageToken

		if len(items) == 0 {
			break
		}

		// Extract playlist snippets
		for _, item := range items {
			allSnippets = append(allSnippets, item.Snippet)
			count++
		}

		if (feed.PlaylistSort != model.SortingDesc && count >= feed.PageSize) || token == "" {
			break
		}
	}

	if len(allSnippets) > feed.PageSize {
		if feed.PlaylistSort != model.SortingDesc {
			allSnippets = allSnippets[:feed.PageSize]
		} else {
			allSnippets = allSnippets[len(allSnippets)-feed.PageSize:]
		}
	}

	snippets := map[string]*youtube.PlaylistItemSnippet{}
	for _, snippet := range allSnippets {
		snippets[snippet.ResourceId.VideoId] = snippet
	}

	// Query video descriptions from the list of ids
	if err := yt.queryVideoDescriptions(ctx, snippets, feed); err != nil {
		return err
	}

	return nil
}

func (yt *YouTubeBuilder) Build(ctx context.Context, cfg *feed.Config) (*model.Feed, error) {
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
	}

	if _feed.PageSize == 0 {
		_feed.PageSize = maxYoutubeResults
	}

	// Query general information about feed (title, description, lang, etc)
	if err := yt.queryFeed(ctx, _feed, &info); err != nil {
		return nil, err
	}

	if err := yt.queryItems(ctx, _feed); err != nil {
		return nil, err
	}

	// YT API client gets 50 episodes per query.
	// Round up to page size.
	if len(_feed.Episodes) > _feed.PageSize {
		_feed.Episodes = _feed.Episodes[:_feed.PageSize]
	}

	sort.Slice(_feed.Episodes, func(i, j int) bool {
		item1, _ := strconv.Atoi(_feed.Episodes[i].Order)
		item2, _ := strconv.Atoi(_feed.Episodes[j].Order)
		return item1 < item2
	})

	return _feed, nil
}

func NewYouTubeBuilder(key string) (*YouTubeBuilder, error) {
	if key == "" {
		return nil, errors.New("empty YouTube API key")
	}

	yt, err := youtube.New(&http.Client{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create youtube client")
	}

	return &YouTubeBuilder{client: yt, key: apiKey(key)}, nil
}
