package models

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
	"git.kirsle.net/apps/barertc/pkg/log"
	"git.kirsle.net/apps/barertc/pkg/messages"
	"gorm.io/gorm"
)

type DirectMessage struct {
	MessageID int64  `gorm:"primaryKey"`
	ChannelID string `gorm:"index"`
	Username  string `gorm:"index"`
	Message   string
	Timestamp int64          // deprecated
	CreatedAt time.Time      `gorm:"index"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

const DirectMessagePerPage = 20

// MigrateV2 upgrades the DirectMessage table for the V2 schema, where we switched
// from using SQLite to GORM so we can optionally use Postgres instead.
//
// During this switch, we also deprecate the old Timestamp column in favor of CreatedAt.
// This function will run in a background goroutine and update legacy rows to the new format.
func (db DirectMessage) MigrateV2() {
	// Find rows that need upgrading.
	var page int
	for {
		page++
		var rows = []*DirectMessage{}
		res := DB.Model(&DirectMessage{}).Where(
			"timestamp > 0",
		).Limit(1000).Find(&rows)
		if res.Error != nil {
			log.Error("DirectMessage.MigrateV2: %s", res.Error)
			return
		}

		if len(rows) == 0 {
			break
		}

		log.Warn("DirectMessage.MigrateV2: Updating %d DMs (page %d)", len(rows), page)
		for _, row := range rows {
			var created = time.Unix(row.Timestamp, 0)
			row.CreatedAt = created
			row.Timestamp = 0
			DB.Save(row)
		}
	}
}

// Init runs initialization tasks for the DMs table (migrate V2 and expire old rows).
func (dm DirectMessage) Init() error {
	if DB == nil {
		return ErrNotInitialized
	}

	// Migrate old rows to the V2 schema.
	dm.MigrateV2()

	// Delete old messages past the retention period.
	if days := config.Current.DirectMessageHistory.RetentionDays; days > 0 {
		cutoff := time.Now().Add(time.Duration(-days) * 24 * time.Hour)
		log.Info("Deleting old DM history past %d days (cutoff: %s)", days, cutoff.Format(time.RFC3339))
		res := DB.Exec(
			"DELETE FROM direct_messages WHERE created_at IS NOT NULL AND created_at < ?",
			cutoff,
		)
		if res.Error != nil {
			return fmt.Errorf("deleting old DMs past the cutoff period: %s", res.Error)
		}
	}

	return nil
}

// LogMessage adds a message to the DM history between two users.
func (dm DirectMessage) LogMessage(fromUsername, toUsername string, msg messages.Message) error {
	if DB == nil {
		return ErrNotInitialized
	}

	if msg.MessageID == 0 {
		return errors.New("message did not have a MessageID")
	}

	var (
		channelID = CreateChannelID(fromUsername, toUsername)
	)

	m := &DirectMessage{
		MessageID: msg.MessageID,
		ChannelID: channelID,
		Username:  fromUsername,
		Message:   msg.Message,
	}
	return DB.Create(&m).Error
}

// ClearMessages clears all stored DMs that the username as a participant in.
func (dm DirectMessage) ClearMessages(username string) (int, error) {
	if DB == nil {
		return 0, ErrNotInitialized
	}

	var placeholders = []interface{}{
		time.Now(),
		fmt.Sprintf("@%s:%%", username), // `@alice:%`
		fmt.Sprintf("%%:@%s", username), // `%:@alice`
		username,
	}

	// Count all the messages we'll delete.
	var (
		count int
		row   = DB.Raw(`
			SELECT COUNT(message_id)
			FROM direct_messages
			WHERE (channel_id LIKE ? OR channel_id LIKE ?)
			OR username = ?
		`, placeholders...)
	)
	if res := row.Scan(&count); res.Error != nil && false {
		return 0, res.Error
	}

	// Delete them all.
	res := DB.Exec(`
		UPDATE direct_messages
		SET deleted_at = ?
		WHERE (channel_id LIKE ? OR channel_id LIKE ?)
		OR username = ?
	`, placeholders...)

	return count, res.Error
}

// TakebackMessage removes a message by its MID from the DM history.
//
// Because the MessageID may have been from a previous chat session, the server can't immediately
// verify the current user had permission to take it back. This function instead will check whether
// a DM history exists sent by this username for that messageID, and if so, returns a
// boolean true that the username/messageID matched which will satisfy the permission check
// in the OnTakeback handler.
func (dm DirectMessage) TakebackMessage(username string, messageID int64, isAdmin bool) (bool, error) {
	if DB == nil {
		return false, ErrNotInitialized
	}

	// Does this messageID exist as sent by the user?
	if !isAdmin {
		var (
			row = DB.Raw(
				"SELECT message_id FROM direct_messages WHERE username = ? AND message_id = ?",
				username, messageID,
			)
			foundMsgID int64
			err        = row.Scan(&foundMsgID)
		)
		if err != nil {
			return false, errors.New("no such message ID found as owned by that user")
		}
	}

	// Delete it.
	res := DB.Exec(
		"DELETE FROM direct_messages WHERE message_id = ?",
		messageID,
	)

	// Return that it was successfully validated and deleted.
	return res.Error == nil, res.Error
}

// PaginateDirectMessages returns a page of messages, the count of remaining, and an error.
func PaginateDirectMessages(fromUsername, toUsername string, beforeID int64) ([]messages.Message, int, error) {
	if DB == nil {
		return nil, 0, ErrNotInitialized
	}

	var (
		result    = []messages.Message{}
		channelID = CreateChannelID(fromUsername, toUsername)
		rows      = []*DirectMessage{}

		// Compute the remaining messages after finding the final messageID this page.
		lastMessageID int64
		remaining     int
	)

	if beforeID == 0 {
		beforeID = math.MaxInt64
	}

	res := DB.Model(&DirectMessage{}).Where(
		"channel_id = ? AND message_id < ?",
		channelID, beforeID,
	).Order("message_id DESC").Limit(DirectMessagePerPage).Find(&rows)
	if res.Error != nil {
		return nil, 0, res.Error
	}

	for _, row := range rows {
		msg := messages.Message{
			MessageID: row.MessageID,
			Username:  row.Username,
			Message:   row.Message,
			Timestamp: row.CreatedAt.Format(time.RFC3339),
		}
		result = append(result, msg)
		lastMessageID = msg.MessageID
	}

	// Get a count of the remaining messages.
	row := DB.Raw(`
		SELECT COUNT(message_id)
		FROM direct_messages
		WHERE channel_id = ?
		AND message_id < ?
	`, channelID, lastMessageID)
	if res := row.Scan(&remaining); res.Error != nil {
		return nil, 0, res.Error
	}

	return result, remaining, nil
}

// PaginateUsernames returns a page of usernames that the current user has conversations with.
//
// Returns the usernames, total count, pages, and error.
func PaginateUsernames(fromUsername, sortBy string, page, perPage int) ([]string, int, int, error) {
	if DB == nil {
		return nil, 0, 0, ErrNotInitialized
	}

	var (
		result  = []string{}
		count   int // Total count of usernames
		pages   int // Number of pages available
		offset  = (page - 1) * perPage
		orderBy string

		// Channel IDs.
		channelIDs = []string{
			fmt.Sprintf(`@%s:%%`, fromUsername),
			fmt.Sprintf(`%%:@%s`, fromUsername),
		}
	)

	if offset < 0 {
		offset = 0
	}

	switch sortBy {
	case "a-z":
		orderBy = "username ASC"
	case "z-a":
		orderBy = "username DESC"
	case "oldest":
		orderBy = "newest_time ASC"
	default:
		orderBy = "newest_time DESC"
	}

	// Get all our distinct channel IDs to filter the query down: otherwise doing an ORDER BY timestamp
	// causes a full table scan index which is very inefficient!
	channelIDs, err := GetDistinctChannelIDs(fromUsername)
	if err != nil {
		return nil, 0, 0, err
	}

	// No channel IDs = no response to fetch.
	if len(channelIDs) == 0 {
		return nil, 0, 0, errors.New("you have no direct messages stored on this chat server")
	}

	type record struct {
		Username string
		// NewestTime time.Time
		// OldestTime time.Time
	}
	var records []record

	// Get all usernames and their newest/oldest timestamps.
	res := DB.Model(&DirectMessage{}).Select(`
		username,
		MAX(created_at) AS newest_time
	`).Where(
		"channel_id IN ? AND username <> ?",
		channelIDs, fromUsername,
	).Group("username").Order(orderBy).Offset(offset).Limit(perPage).Scan(&records)
	if res.Error != nil {
		return nil, 0, 0, res.Error
	}

	for _, row := range records {
		result = append(result, row.Username)
	}

	// The count of distinct channel IDs earlier.
	count = len(channelIDs)

	pages = int(math.Ceil(float64(count) / float64(perPage)))
	if pages < 1 {
		pages = 1
	}

	return result, count, pages, nil
}

// GetDistinctChannelIDs collects all of the conversation thread IDs the current user is a party to.
func GetDistinctChannelIDs(username string) ([]string, error) {
	var (
		result     = []string{}
		channelIDs = []string{
			fmt.Sprintf(`@%s:%%`, username),
			fmt.Sprintf(`%%:@%s`, username),
		}
	)

	res := DB.Model(&DirectMessage{}).Select(
		"DISTINCT(channel_id)",
	).Where(
		"(channel_id LIKE ? OR channel_id LIKE ?)",
		channelIDs[0], channelIDs[1],
	).Scan(&result)
	if res.Error != nil {
		return nil, res.Error
	}

	return result, nil
}

// CreateChannelID returns a deterministic channel ID for a direct message conversation.
//
// The usernames (passed in any order) are sorted alphabetically and composed into the channel ID.
func CreateChannelID(fromUsername, toUsername string) string {
	var parts = []string{fromUsername, toUsername}
	sort.Strings(parts)
	return fmt.Sprintf(
		"@%s:@%s",
		parts[0],
		parts[1],
	)
}
