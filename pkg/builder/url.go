package builder

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"

	"github.com/mxpv/podsync/pkg/model"
)

func ParseURL(link string) (model.Info, error) {
	parsed, err := parseURL(link)
	if err != nil {
		return model.Info{}, err
	}

	info := model.Info{}

	if strings.HasSuffix(parsed.Host, "bilibili.com") {
		kind, id, err := parseBilibiliURL(parsed)
		if err != nil {
			return model.Info{}, err
		}

		info.Provider = model.ProviderBilibili
		info.LinkType = kind
		info.ItemID = id

		return info, nil
	}

	if strings.HasSuffix(parsed.Host, "youtube.com") {
		kind, id, err := parseYoutubeURL(parsed)
		if err != nil {
			return model.Info{}, err
		}

		info.Provider = model.ProviderYoutube
		info.LinkType = kind
		info.ItemID = id

		return info, nil
	}

	if strings.HasSuffix(parsed.Host, "vimeo.com") {
		kind, id, err := parseVimeoURL(parsed)
		if err != nil {
			return model.Info{}, err
		}

		info.Provider = model.ProviderVimeo
		info.LinkType = kind
		info.ItemID = id

		return info, nil
	}

	if strings.HasSuffix(parsed.Host, "soundcloud.com") {
		kind, id, err := parseSoundcloudURL(parsed)
		if err != nil {
			return model.Info{}, err
		}

		info.Provider = model.ProviderSoundcloud
		info.LinkType = kind
		info.ItemID = id

		return info, nil
	}

	return model.Info{}, errors.New("unsupported URL host")
}

func parseURL(link string) (*url.URL, error) {
	if !strings.HasPrefix(link, "http") {
		link = "https://" + link
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse url: %s", link)
	}

	return parsed, nil
}

func parseYoutubeURL(parsed *url.URL) (model.Type, string, error) {
	path := parsed.EscapedPath()

	// https://www.youtube.com/playlist?list=PLCB9F975ECF01953C
	// https://www.youtube.com/watch?v=rbCbho7aLYw&list=PLMpEfaKcGjpWEgNtdnsvLX6LzQL0UC0EM
	if strings.HasPrefix(path, "/playlist") || strings.HasPrefix(path, "/watch") {
		kind := model.TypePlaylist

		id := parsed.Query().Get("list")
		if id != "" {
			return kind, id, nil
		}

		return "", "", errors.New("invalid playlist link")
	}

	// - https://www.youtube.com/channel/UC5XPnUk8Vvv_pWslhwom6Og
	// - https://www.youtube.com/channel/UCrlakW-ewUT8sOod6Wmzyow/videos
	if strings.HasPrefix(path, "/channel") {
		kind := model.TypeChannel
		parts := strings.Split(parsed.EscapedPath(), "/")
		if len(parts) <= 2 {
			return "", "", errors.New("invalid youtube channel link")
		}

		id := parts[2]
		if id == "" {
			return "", "", errors.New("invalid id")
		}

		return kind, id, nil
	}

	// - https://www.youtube.com/user/fxigr1
	if strings.HasPrefix(path, "/user") {
		kind := model.TypeUser

		parts := strings.Split(parsed.EscapedPath(), "/")
		if len(parts) <= 2 {
			return "", "", errors.New("invalid user link")
		}

		id := parts[2]
		if id == "" {
			return "", "", errors.New("invalid id")
		}

		return kind, id, nil
	}

	return "", "", errors.New("unsupported link format")
}

func parseBilibiliURL(parsed *url.URL) (model.Type, string, error) {
	// https://space.bilibili.com/7380321
	// https://space.bilibili.com/7380321/channel/collectiondetail?sid=531853

	subdomain := strings.Split(parsed.Host, ".")[0]
	parts := strings.Split(parsed.EscapedPath(), "/")

	if len(parts) <= 1 || subdomain != "space" {
		return "", "", errors.New("invalid bilibili link path")
	}
	var kind model.Type
	if len(parts) == 2 {
		kind = model.TypeUser
		return kind, parts[1], nil
	} else if parts[2] == "channel" {
		kind = model.TypeChannel
		params, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			return "", "", errors.New("invalid bilibili channel path")
		}
		return kind, parts[1] + ":" + params["sid"][0], nil
	}
	return "", "", errors.New("unsupported link format")
}

func parseVimeoURL(parsed *url.URL) (model.Type, string, error) {
	parts := strings.Split(parsed.EscapedPath(), "/")
	if len(parts) <= 1 {
		return "", "", errors.New("invalid vimeo link path")
	}

	var kind model.Type
	switch parts[1] {
	case "groups":
		kind = model.TypeGroup
	case "channels":
		kind = model.TypeChannel
	default:
		kind = model.TypeUser
	}

	if kind == model.TypeGroup || kind == model.TypeChannel {
		if len(parts) <= 2 {
			return "", "", errors.New("invalid channel link")
		}

		id := parts[2]
		if id == "" {
			return "", "", errors.New("invalid id")
		}

		return kind, id, nil
	}

	if kind == model.TypeUser {
		id := parts[1]
		if id == "" {
			return "", "", errors.New("invalid id")
		}

		return kind, id, nil
	}

	return "", "", errors.New("unsupported link format")
}

func parseSoundcloudURL(parsed *url.URL) (model.Type, string, error) {
	parts := strings.Split(parsed.EscapedPath(), "/")
	if len(parts) <= 3 {
		return "", "", errors.New("invald soundcloud link path")
	}

	var kind model.Type

	// - https://soundcloud.com/user/sets/example-set
	switch parts[2] {
	case "sets":
		kind = model.TypePlaylist
	default:
		return "", "", errors.New("invalid soundcloud url, missing sets")
	}

	id := parts[3]

	return kind, id, nil
}
