package model

type Type string

const (
	TypeChannel  = Type("channel")
	TypePlaylist = Type("playlist")
	TypeUser     = Type("user")
	TypeGroup    = Type("group")
)

type Provider string

const (
	ProviderYoutube    = Provider("youtube")
	ProviderVimeo      = Provider("vimeo")
	ProviderSoundcloud = Provider("soundcloud")
	ProviderTwitch     = Provider("twitch")
)

// Info represents data extracted from URL
type Info struct {
	LinkType Type     // Either group, channel or user
	Provider Provider // Youtube, Vimeo, SoundCloud or Twitch
	ItemID   string
}
