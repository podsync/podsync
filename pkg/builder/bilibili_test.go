package builder

import (
	"testing"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBilibili_BuildFeed(t *testing.T) {
	builder, err := NewBilibiliBuilder()
	require.NoError(t, err)

	urls := []string{
		"https://space.bilibili.com/1302298364",
		"https://space.bilibili.com/397490386/channel/seriesdetail?sid=1203833",
	}

	t.Run(urls[0], func(t *testing.T) {
		feed, err := builder.Build(testCtx, &feed.Config{URL: urls[0]})
		require.NoError(t, err)

		assert.NotEmpty(t, feed.Title)
		assert.NotEmpty(t, feed.Description)
		assert.NotEmpty(t, feed.Author)
		assert.NotEmpty(t, feed.ItemURL)

		assert.NotZero(t, len(feed.Episodes))

		for _, item := range feed.Episodes {
			assert.NotEmpty(t, item.Title)
			assert.NotEmpty(t, item.VideoURL)
			assert.NotZero(t, item.Duration)
			assert.NotEmpty(t, item.Title)
			assert.NotEmpty(t, item.Thumbnail)
		}
	})

	t.Run(urls[1], func(t *testing.T) {
		_, err := builder.Build(testCtx, &feed.Config{URL: urls[1]})
		require.Error(t, err, "Bilibili channel not supported.")
	})
}
