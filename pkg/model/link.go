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
	ProviderBilibili   = Provider("bilibili")
	ProviderYoutube    = Provider("youtube")
	ProviderVimeo      = Provider("vimeo")
	ProviderSoundcloud = Provider("soundcloud")
)

// Info represents data extracted from URL
type Info struct {
	LinkType Type     // Either group, channel or user
	Provider Provider // Youtube, Vimeo, or SoundCloud
	ItemID   string
}
