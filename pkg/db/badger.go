package db

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/mxpv/podsync/pkg/model"
)

const (
	versionPath   = "podsync/version"
	feedPrefix    = "feed/"
	feedPath      = "feed/%s"
	episodePrefix = "episode/%s/"
	episodePath   = "episode/%s/%s" // FeedID + EpisodeID
)

// BadgerConfig represents BadgerDB configuration parameters
type BadgerConfig struct {
	Truncate bool `toml:"truncate"`
	FileIO   bool `toml:"file_io"`
}

type Badger struct {
	db *badger.DB
}

var _ Storage = (*Badger)(nil)

func NewBadger(config *Config) (*Badger, error) {
	var (
		dir = config.Dir
	)

	log.Infof("opening database %q", dir)

	// Make sure database directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.Wrap(err, "could not mkdir database dir")
	}

	opts := badger.DefaultOptions(dir).
		WithLogger(log.StandardLogger()).
		WithTruncate(true)

	if config.Badger != nil {
		opts.Truncate = config.Badger.Truncate
		if config.Badger.FileIO {
			opts.ValueLogLoadingMode = options.FileIO
		}
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open database")
	}

	storage := &Badger{db: db}

	if err := db.Update(func(txn *badger.Txn) error {
		if err := storage.setObj(txn, []byte(versionPath), CurrentVersion, false); err != nil && err != model.ErrAlreadyExists {
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to read database version")
	}

	return &Badger{db: db}, nil
}

func (b *Badger) Close() error {
	log.Debug("closing database")
	return b.db.Close()
}

func (b *Badger) Version() (int, error) {
	var (
		version = -1
	)

	err := b.db.View(func(txn *badger.Txn) error {
		return b.getObj(txn, []byte(versionPath), &version)
	})

	return version, err
}

func (b *Badger) AddFeed(_ context.Context, feedID string, feed *model.Feed) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Insert or update feed info
		feedKey := b.getKey(feedPath, feedID)
		if err := b.setObj(txn, feedKey, feed, true); err != nil {
			return err
		}

		// Append new episodes
		for _, episode := range feed.Episodes {
			episodeKey := b.getKey(episodePath, feedID, episode.ID)
			err := b.setObj(txn, episodeKey, episode, false)
			if !(err == nil || err == model.ErrAlreadyExists) {
				return errors.Wrapf(err, "failed to save episode %q", feedID)
			}
		}

		return nil
	})
}

func (b *Badger) GetFeed(_ context.Context, feedID string) (*model.Feed, error) {
	var (
		feed    = model.Feed{}
		feedKey = b.getKey(feedPath, feedID)
	)

	if err := b.db.View(func(txn *badger.Txn) error {
		// Query feed
		if err := b.getObj(txn, feedKey, &feed); err != nil {
			return err
		}

		// Query episodes
		if err := b.walkEpisodes(txn, feedID, func(episode *model.Episode) error {
			feed.Episodes = append(feed.Episodes, episode)
			return nil
		}); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &feed, nil
}

func (b *Badger) WalkFeeds(_ context.Context, cb func(feed *model.Feed) error) error {
	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = b.getKey(feedPrefix)
		opts.PrefetchValues = true
		return b.iterator(txn, opts, func(item *badger.Item) error {
			feed := &model.Feed{}
			if err := b.unmarshalObj(item, feed); err != nil {
				return err
			}

			return cb(feed)
		})
	})
}

func (b *Badger) DeleteFeed(_ context.Context, feedID string) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Feed
		feedKey := b.getKey(feedPath, feedID)
		if err := txn.Delete(feedKey); err != nil {
			return errors.Wrapf(err, "failed to delete feed %q", feedID)
		}

		// Episodes
		opts := badger.DefaultIteratorOptions
		opts.Prefix = b.getKey(episodePrefix, feedID)
		opts.PrefetchValues = false
		if err := b.iterator(txn, opts, func(item *badger.Item) error {
			return txn.Delete(item.KeyCopy(nil))
		}); err != nil {
			return errors.Wrapf(err, "failed to iterate episodes for feed %q", feedID)
		}

		return nil
	})
}

func (b *Badger) GetEpisode(_ context.Context, feedID string, episodeID string) (*model.Episode, error) {
	var (
		episode model.Episode
		err     error
		key     = b.getKey(episodePath, feedID, episodeID)
	)

	err = b.db.View(func(txn *badger.Txn) error {
		return b.getObj(txn, key, &episode)
	})

	return &episode, err
}

func (b *Badger) UpdateEpisode(feedID string, episodeID string, cb func(episode *model.Episode) error) error {
	var (
		key     = b.getKey(episodePath, feedID, episodeID)
		episode model.Episode
	)

	return b.db.Update(func(txn *badger.Txn) error {
		if err := b.getObj(txn, key, &episode); err != nil {
			return err
		}

		if err := cb(&episode); err != nil {
			return err
		}

		if episode.ID != episodeID {
			return errors.New("can't change episode ID")
		}

		return b.setObj(txn, key, &episode, true)
	})
}

func (b *Badger) DeleteEpisode(feedID, episodeID string) error {
	key := b.getKey(episodePath, feedID, episodeID)
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (b *Badger) WalkEpisodes(_ context.Context, feedID string, cb func(episode *model.Episode) error) error {
	return b.db.View(func(txn *badger.Txn) error {
		return b.walkEpisodes(txn, feedID, cb)
	})
}

func (b *Badger) walkEpisodes(txn *badger.Txn, feedID string, cb func(episode *model.Episode) error) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = b.getKey(episodePrefix, feedID)
	opts.PrefetchValues = true
	return b.iterator(txn, opts, func(item *badger.Item) error {
		feed := &model.Episode{}
		if err := b.unmarshalObj(item, feed); err != nil {
			return err
		}

		return cb(feed)
	})
}

func (b *Badger) iterator(txn *badger.Txn, opts badger.IteratorOptions, callback func(item *badger.Item) error) error {
	iter := txn.NewIterator(opts)
	defer iter.Close()

	for iter.Rewind(); iter.Valid(); iter.Next() {
		item := iter.Item()

		if err := callback(item); err != nil {
			return err
		}
	}

	return nil
}

func (b *Badger) getKey(format string, a ...interface{}) []byte {
	resourcePath := fmt.Sprintf(format, a...)
	fullPath := fmt.Sprintf("podsync/v%d/%s", CurrentVersion, resourcePath)

	return []byte(fullPath)
}

func (b *Badger) setObj(txn *badger.Txn, key []byte, obj interface{}, overwrite bool) error {
	if !overwrite {
		// Overwrites are not allowed, make sure there is no object with the given key
		_, err := txn.Get(key)
		if err == nil {
			return model.ErrAlreadyExists
		} else if err != badger.ErrKeyNotFound {
			return errors.Wrap(err, "failed to check whether key exists")
		}
	}

	data, err := b.marshalObj(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize object for key %q", key)
	}

	return txn.Set(key, data)
}

func (b *Badger) getObj(txn *badger.Txn, key []byte, out interface{}) error {
	item, err := txn.Get(key)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return model.ErrNotFound
		}

		return err
	}

	return b.unmarshalObj(item, out)
}

func (b *Badger) marshalObj(obj interface{}) ([]byte, error) {
	return json.Marshal(obj)
}

func (b *Badger) unmarshalObj(item *badger.Item, out interface{}) error {
	return item.Value(func(val []byte) error {
		return json.Unmarshal(val, out)
	})
}
