package barertc

import (
	"sync"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
	"git.kirsle.net/apps/barertc/pkg/log"
	"git.kirsle.net/apps/barertc/pkg/messages"
)

// CommunityModerationStore is a feature where the users of the chat room can help to mark cameras
// as 'explicit' when the broadcaster has forgotten to do so, in the absence of an admin user
// who otherwise would be the only person who can set a camera red for a user.
//
// If multiple viewers of your camera have given you a nudge that maybe your camera should be
// marked as red (explicit), the chat server will mark it red for you and temporarily keep it
// locked to red with the red camera moderation rule.
//
// The BareRTC server keeps the community moderation data in process memory so even if the
// broadcaster logs out and back in again, their recent flags or temporary forced red camera
// status will persist until it expires or until the server is turned off and on again.
type CommunityModerationStore struct {
	mu sync.RWMutex

	// Map of usernames and their recent community red camera flags.
	// First key is the target username (the broadcaster).
	// Second map is of the unique flagging usernames and the time of the flag.
	flaggedUsers map[string]map[string]time.Time

	// Map of users currently locked to red camera status with the expiration date.
	forcedRedCameras map[string]time.Time

	// If somebody's camera was marked as red and they repeatedly try and turn it blue,
	// keep track of how many times so we can lengthen the expiration date of their red
	// camera lock to be a little bit longer every time.
	blueCameraAttempts map[string]int
}

// Global storage for community moderation in memory.
var (
	CommunityModeration = CommunityModerationStore{
		flaggedUsers:       map[string]map[string]time.Time{},
		forcedRedCameras:   map[string]time.Time{},
		blueCameraAttempts: map[string]int{},
	}
)

// Special MessageID for the 'React' command when users nudge NSFW on each other's cameras.
const ReactNudgeNsfwMessageID = -451

// Flag a user's blue camera.
//
// When the broadcaster has a blue camera and the viewer thinks it should be marked as red,
// this function is called. If the broadcaster has accumulated enough flags in a period of
// time, their camera will be set to red and locked there for a while.
func (cu *CommunityModerationStore) Flag(s *Server, broadcaster, viewer string) {
	if !config.Current.CommunityModeration.Enabled {
		return
	}

	cu.mu.Lock()

	// Initialize the broadcaster's cache?
	if _, ok := cu.flaggedUsers[broadcaster]; !ok {
		cu.flaggedUsers[broadcaster] = map[string]time.Time{}
	}

	// A new unique flag? (Or the old stored flag is expired?)
	if ts, ok := cu.flaggedUsers[broadcaster][viewer]; !ok || cu.IsExpired(ts) {
		cu.flaggedUsers[broadcaster][viewer] = time.Now()
	}

	cu.mu.Unlock()

	log.Error("CommunityModerationStore: Broadcaster %s was nudged NSFW by viewer %s", broadcaster, viewer)

	// If the broadcaster has accumulated enough flags, mark their camera red.
	if cu.Count(broadcaster) >= config.Current.CommunityModeration.RedCameraFlagsCount {
		cu.Punish(s, broadcaster)
	}
}

// Punish a blue camera broadcaster and set their camera to red because they accumulated enough flags.
func (cu *CommunityModerationStore) Punish(s *Server, broadcaster string) {
	if !config.Current.CommunityModeration.Enabled {
		return
	}

	cu.mu.Lock()

	// Already punished?
	if expires, ok := cu.forcedRedCameras[broadcaster]; ok {
		if time.Now().Before(expires) {
			cu.mu.Unlock()
			return
		}
	}

	// Initialize their punishment state.
	cu.forcedRedCameras[broadcaster] = time.Now().Add(config.Current.CommunityModeration.RedCameraLockExpires * time.Second)
	cu.blueCameraAttempts[broadcaster] = -1
	log.Error("CommunityModerationStore: Username %s is now punished with the red camera rule", broadcaster)

	cu.mu.Unlock()

	// Set the user's camera to red.
	if sub, err := s.GetSubscriber(broadcaster); err == nil {
		sub.ChatServer(
			"<strong>Your camera is now marked as <span class=\"has-text-danger\"><i class=\"fa fa-fire\"></i> Explicit!</span></strong><br><br>" +
				"Just as a friendly reminder, please remember to mark your camera as 'Explicit' using the button at the top of the page " +
				"if you are going to be sexual on webcam.<br><br>Multiple community members have flagged that your camera should have been marked as " +
				"explicit. As such, <strong>your camera has now been marked as Explicit</strong> on your behalf.<br><br>" +
				"Your camera will also be temporarily locked as 'Explicit' for several minutes.",
		)
		sub.VideoStatus |= messages.VideoFlagNSFW
		sub.SendMe()
		s.SendWhoList()
	}
}

// IsPunished checks if the broadcaster is currently punished with the red camera rule.
//
// If their time has expired, it will clean up their data and return false (allowing them to go blue again).
func (cu *CommunityModerationStore) IsPunished(broadcaster string) bool {
	if !config.Current.CommunityModeration.Enabled {
		return false
	}

	cu.mu.Lock()
	defer cu.mu.Unlock()

	expires, ok := cu.forcedRedCameras[broadcaster]
	if !ok {
		return false
	}

	// Off the hook?
	if time.Now().After(expires) {
		log.Error("CommunityModerationStore: Username %s has served their time and is no longer punished", broadcaster)
		delete(cu.forcedRedCameras, broadcaster)
		delete(cu.blueCameraAttempts, broadcaster)
		return false
	}

	return true
}

// PunishedUserTriedBlue is called when a punished user tries to broadcast a blue camera before their punishment has expired.
//
// They get one free attempt to do so, but any subsequent attempt will add more time to their current expiration date.
//
// The algorithm is: 30 seconds times the number of repeated attempts gets added to their expiration time.
func (cu *CommunityModerationStore) PunishedUserTriedBlue(broadcaster string) {
	if !config.Current.CommunityModeration.Enabled {
		return
	}

	cu.mu.Lock()
	defer cu.mu.Unlock()

	expires, ok := cu.forcedRedCameras[broadcaster]
	if !ok {
		return
	}

	// Off the hook?
	if time.Now().After(expires) {
		log.Error("CommunityModerationStore: Username %s has served their time and is no longer punished", broadcaster)
		delete(cu.forcedRedCameras, broadcaster)
		delete(cu.blueCameraAttempts, broadcaster)
		return
	}

	// Increment their attempts counter.
	// Note: it begins at -1 so their first free attempt sets it to zero.
	attempts := cu.blueCameraAttempts[broadcaster]
	attempts++
	cu.blueCameraAttempts[broadcaster] = attempts
	if attempts == 0 {
		return
	}

	// Extend their duration.
	cu.forcedRedCameras[broadcaster] = expires.Add(30 * time.Second * time.Duration(attempts))
	log.Error("CommunityModerationStore: Punished username %s tried turning their camera blue %d times, their new expiration date is %s", broadcaster, attempts, cu.forcedRedCameras[broadcaster])
}

// Count the recent community flags for the broadcaster.
func (cu *CommunityModerationStore) Count(broadcaster string) int {
	if !config.Current.CommunityModeration.Enabled {
		return 0
	}

	cu.mu.Lock()
	defer cu.mu.Unlock()

	// Count all of the non-expired flags.
	var (
		viewers        = []string{}
		expiredViewers = []string{}
	)
	if vm, ok := cu.flaggedUsers[broadcaster]; !ok {
		return 0
	} else {
		for viewer, ts := range vm {
			if cu.IsExpired(ts) {
				expiredViewers = append(expiredViewers, viewer)
			} else {
				viewers = append(viewers, viewer)
			}
		}
	}

	// Clean up any expired usernames.
	for _, viewer := range expiredViewers {
		delete(cu.flaggedUsers[broadcaster], viewer)
	}

	log.Info("CommunityModerationStore: broadcaster %s has currently %d nudge nsfw flags", broadcaster, len(viewers))
	return len(viewers)
}

// IsExpired checks whether a community flag has expired.
func (cu *CommunityModerationStore) IsExpired(ts time.Time) bool {
	if !config.Current.CommunityModeration.Enabled {
		return false
	}

	return time.Since(ts) > (config.Current.CommunityModeration.RedCameraFlagsWindow * time.Second)
}
