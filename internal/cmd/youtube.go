package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const youtubeCommentsOAuthScope = "https://www.googleapis.com/auth/youtube.force-ssl"

type YouTubeCmd struct {
	Activities YouTubeActivitiesCmd `cmd:"" name:"activities" aliases:"activity" help:"List channel activities"`
	Videos     YouTubeVideosCmd     `cmd:"" name:"videos" aliases:"video" help:"List or get videos"`
	Playlists  YouTubePlaylistsCmd  `cmd:"" name:"playlists" aliases:"playlist" help:"List playlists"`
	Comments   YouTubeCommentsCmd   `cmd:"" name:"comments" aliases:"comment" help:"List comment threads"`
	Channels   YouTubeChannelsCmd   `cmd:"" name:"channels" aliases:"channel" help:"List channels"`
	Search     YouTubeSearchCmd     `cmd:"" name:"search" aliases:"find" help:"Search YouTube for videos, channels, or playlists"`
}

type YouTubeActivitiesCmd struct {
	List YouTubeActivitiesListCmd `cmd:"" name:"list" aliases:"ls" help:"List activities for a channel (or authenticated user)"`
}

type YouTubeActivitiesListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID"`
	Mine      bool   `name:"mine" help:"Use authenticated user's channel (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeActivitiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" && !c.Mine {
		return usage("set --channel-id ID or --mine (--mine requires -a account)")
	}
	if channelID != "" && c.Mine {
		return usage("use either --channel-id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	call := svc.Activities.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID != "" {
		call = call.ChannelId(channelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No activities")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeActivityColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeVideosCmd struct {
	List YouTubeVideosListCmd `cmd:"" name:"list" aliases:"ls" help:"List videos by ID or chart"`
}

type YouTubeVideosListCmd struct {
	ID     string `name:"id" help:"Comma-separated video IDs"`
	Chart  string `name:"chart" help:"Chart: mostPopular (regionCode required)"`
	Region string `name:"region" help:"Region code (e.g. US) for chart"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page   string `name:"page" help:"Page token"`
}

func (c *YouTubeVideosListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	ids := splitCSV(c.ID)
	chart := strings.TrimSpace(c.Chart)
	region := strings.TrimSpace(c.Region)
	if len(ids) == 0 && chart == "" {
		return usage("set --id VIDEO_IDS or --chart mostPopular")
	}
	if len(ids) > 0 && chart != "" {
		return usage("use either --id or --chart, not both")
	}
	if chart != "" && chart != "mostPopular" {
		return usage("--chart must be mostPopular")
	}
	if chart == "mostPopular" && region == "" {
		return usage("--chart mostPopular requires --region (e.g. US)")
	}

	svc, err := getYouTubeReadService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Videos.List([]string{"snippet", "contentDetails", "statistics"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if len(ids) > 0 {
		call = call.Id(ids...)
	} else {
		call = call.Chart(chart).RegionCode(region)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No videos")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeVideoColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubePlaylistsCmd struct {
	List YouTubePlaylistsListCmd `cmd:"" name:"list" aliases:"ls" help:"List playlists by channel or authenticated user"`
}

type YouTubePlaylistsListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID"`
	Mine      bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubePlaylistsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" && !c.Mine {
		return usage("set --channel-id ID or --mine (--mine requires -a account)")
	}
	if channelID != "" && c.Mine {
		return usage("use either --channel-id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	call := svc.Playlists.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID != "" {
		call = call.ChannelId(channelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No playlists")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubePlaylistColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeCommentsCmd struct {
	List YouTubeCommentsListCmd `cmd:"" name:"list" aliases:"ls" help:"List comment threads for a video or channel"`
}

type YouTubeCommentsListCmd struct {
	VideoID   string `name:"video-id" help:"Video ID (list top-level comments for this video)"`
	ChannelID string `name:"channel-id" help:"Channel ID (list comments that mention the channel)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeCommentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	videoID := strings.TrimSpace(c.VideoID)
	channelID := strings.TrimSpace(c.ChannelID)
	if videoID == "" && channelID == "" {
		return usage("set --video-id ID or --channel-id ID")
	}
	if videoID != "" && channelID != "" {
		return usage("use either --video-id or --channel-id, not both")
	}

	svc, err := getYouTubeCommentsService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.CommentThreads.List([]string{"snippet"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if videoID != "" {
		call = call.VideoId(videoID)
	} else {
		call = call.ChannelId(channelID)
	}
	resp, err := call.Do()
	if err != nil {
		return wrapYouTubeCommentsError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No comment threads")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeCommentColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeChannelsCmd struct {
	List YouTubeChannelsListCmd `cmd:"" name:"list" aliases:"ls" help:"List channels by ID or authenticated user"`
}

type YouTubeChannelsListCmd struct {
	ID   string `name:"id" help:"Comma-separated channel IDs"`
	Mine bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page string `name:"page" help:"Page token"`
}

func (c *YouTubeChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	ids := splitCSV(c.ID)
	if len(ids) == 0 && !c.Mine {
		return usage("set --id CHANNEL_IDS or --mine (--mine requires -a account)")
	}
	if len(ids) > 0 && c.Mine {
		return usage("use either --id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet", "statistics", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if len(ids) > 0 {
		call = call.Id(ids...)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No channels")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeChannelColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeSearchCmd struct {
	List YouTubeSearchListCmd `cmd:"" name:"list" aliases:"ls" help:"Search for videos, channels, or playlists"`
}

type YouTubeSearchListCmd struct {
	Query     string `arg:"" help:"Search query"`
	Type      string `name:"type" help:"Resource type: video, channel, playlist (comma-separated)" default:"video"`
	Order     string `name:"order" help:"Sort order: relevance, date, rating, title, videoCount, viewCount" default:"relevance" enum:"relevance,date,rating,title,videoCount,viewCount"`
	ChannelID string `name:"channel-id" help:"Restrict results to a specific channel"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeSearchListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	query := strings.TrimSpace(c.Query)
	if query == "" {
		return usage("search query is required")
	}

	types := splitCSV(c.Type)
	if len(types) == 0 {
		return usage("--type must be video, channel, or playlist (comma-separated)")
	}
	for _, t := range types {
		switch t {
		case "video", "channel", "playlist":
		default:
			return usage("--type must be video, channel, or playlist (comma-separated)")
		}
	}

	svc, err := getYouTubeReadService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Search.List([]string{"snippet"}).
		Q(query).
		Type(types...).
		Order(c.Order).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID := strings.TrimSpace(c.ChannelID); channelID != "" {
		call = call.ChannelId(channelID)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}
	resp.Items = filterYouTubeSearchItemsByType(resp.Items, types)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No results")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeSearchColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func validateYouTubeMax(limit int64) error {
	if limit < 1 || limit > 50 {
		return usage("--max must be between 1 and 50")
	}
	return nil
}

func youtubeItemsOrEmpty[T any](items []*T) []*T {
	if items == nil {
		return []*T{}
	}
	return items
}

func filterYouTubeSearchItemsByType(items []*youtube.SearchResult, allowed []string) []*youtube.SearchResult {
	if len(items) == 0 || len(allowed) == 0 {
		return items
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, typ := range allowed {
		allowedSet[typ] = true
	}
	filtered := items[:0]
	for _, item := range items {
		if allowedSet[youtubeSearchResultType(item)] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func youtubeSearchResultType(item *youtube.SearchResult) string {
	if item == nil || item.Id == nil {
		return ""
	}
	switch {
	case item.Id.VideoId != "":
		return "video"
	case item.Id.ChannelId != "":
		return "channel"
	case item.Id.PlaylistId != "":
		return "playlist"
	default:
		return ""
	}
}

func getYouTubeReadService(ctx context.Context, flags *RootFlags) (*youtube.Service, error) {
	if youtubeAccountSelectorPresent(flags) {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		return getYouTubeServiceForAccount(ctx, account)
	}
	return getYouTubeServiceWithAPIKey(ctx)
}

func getYouTubeCommentsService(ctx context.Context, flags *RootFlags) (*youtube.Service, error) {
	if youtubeAccountSelectorPresent(flags) {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		return getYouTubeCommentsServiceForAccount(ctx, account)
	}
	return getYouTubeServiceWithAPIKey(ctx)
}

func youtubeAccountSelectorPresent(flags *RootFlags) bool {
	return flagAccount(flags) != "" || strings.TrimSpace(os.Getenv("GOG_ACCOUNT")) != "" || hasDirectAccessToken(flags)
}

func wrapYouTubeCommentsError(err error, flags *RootFlags) error {
	if err == nil {
		return nil
	}
	errText := err.Error()
	if !strings.Contains(errText, "insufficientPermissions") &&
		!strings.Contains(errText, "insufficient authentication scopes") &&
		!strings.Contains(errText, "ACCESS_TOKEN_SCOPE_INSUFFICIENT") {
		return err
	}
	if !youtubeAccountSelectorPresent(flags) {
		return err
	}
	account, accountErr := requireAccount(flags)
	if accountErr != nil {
		return err
	}
	return errfmt.NewUserFacingError(
		fmt.Sprintf("youtube comments OAuth requires %s; re-authenticate with: gog auth add %s --services youtube --extra-scopes %s --force-consent", youtubeCommentsOAuthScope, account, youtubeCommentsOAuthScope),
		err,
	)
}
