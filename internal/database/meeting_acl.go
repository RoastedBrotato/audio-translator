package database

import (
	"database/sql"
	"fmt"
	"time"
)

// MeetingACLEntry represents an access control entry for a meeting
type MeetingACLEntry struct {
	ID        int       `json:"id"`
	MeetingID string    `json:"meetingId"`
	UserID    int       `json:"userId"`
	Role      string    `json:"role"`
	GrantedBy *int      `json:"grantedBy,omitempty"`
	GrantedAt time.Time `json:"grantedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Additional fields for API responses
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// Role hierarchy levels for comparison
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// roleLevel returns the numeric level of a role (higher is more privileged)
func roleLevel(role string) int {
	switch role {
	case RoleOwner:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// GetUserMeetingRole returns the role a user has for a meeting
// Returns "owner" if user is the meeting creator, otherwise checks ACL table
// Returns empty string if user has no access
func GetUserMeetingRole(userID int, meetingID string) (string, error) {
	// First check if user is the meeting creator (automatic owner)
	var createdBy sql.NullInt64
	err := DB.QueryRow(`SELECT created_by FROM meetings WHERE id = $1`, meetingID).Scan(&createdBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Meeting doesn't exist
		}
		return "", fmt.Errorf("failed to check meeting creator: %w", err)
	}

	if createdBy.Valid && int(createdBy.Int64) == userID {
		return RoleOwner, nil
	}

	// Check ACL table for explicit role assignment
	var role string
	err = DB.QueryRow(`
		SELECT role FROM meeting_access_control
		WHERE meeting_id = $1 AND user_id = $2
	`, meetingID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil // No access
	}
	if err != nil {
		return "", fmt.Errorf("failed to get user meeting role: %w", err)
	}

	return role, nil
}

// UserHasMinimumRole checks if a user has at least the required role level
// Role hierarchy: owner > editor > viewer
func UserHasMinimumRole(userID int, meetingID string, requiredRole string) (bool, error) {
	userRole, err := GetUserMeetingRole(userID, meetingID)
	if err != nil {
		return false, err
	}

	if userRole == "" {
		return false, nil // No access
	}

	return roleLevel(userRole) >= roleLevel(requiredRole), nil
}

// GrantMeetingAccess grants or updates access for a user to a meeting
// If the user already has an ACL entry, their role is updated
// Cannot grant owner role or modify creator's access
func GrantMeetingAccess(meetingID string, userID int, role string, grantedBy int) error {
	// Validate role
	if role != RoleEditor && role != RoleViewer {
		return fmt.Errorf("invalid role: can only grant 'editor' or 'viewer' roles")
	}

	// Check if user is the meeting creator (cannot add ACL for creator)
	var createdBy sql.NullInt64
	err := DB.QueryRow(`SELECT created_by FROM meetings WHERE id = $1`, meetingID).Scan(&createdBy)
	if err == sql.ErrNoRows {
		return fmt.Errorf("meeting not found")
	}
	if err != nil {
		return fmt.Errorf("failed to check meeting creator: %w", err)
	}

	if createdBy.Valid && int(createdBy.Int64) == userID {
		return fmt.Errorf("cannot modify creator's access (creators are owners by definition)")
	}

	// Insert or update the ACL entry
	query := `
		INSERT INTO meeting_access_control (meeting_id, user_id, role, granted_by, granted_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (meeting_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by, updated_at = NOW()
	`
	_, err = DB.Exec(query, meetingID, userID, role, grantedBy)
	if err != nil {
		return fmt.Errorf("failed to grant meeting access: %w", err)
	}

	return nil
}

// RevokeMeetingAccess removes access for a user from a meeting
// Cannot revoke creator's access
func RevokeMeetingAccess(meetingID string, userID int) error {
	// Check if user is the meeting creator (cannot revoke creator's access)
	var createdBy sql.NullInt64
	err := DB.QueryRow(`SELECT created_by FROM meetings WHERE id = $1`, meetingID).Scan(&createdBy)
	if err == sql.ErrNoRows {
		return fmt.Errorf("meeting not found")
	}
	if err != nil {
		return fmt.Errorf("failed to check meeting creator: %w", err)
	}

	if createdBy.Valid && int(createdBy.Int64) == userID {
		return fmt.Errorf("cannot revoke creator's access")
	}

	// Delete the ACL entry
	result, err := DB.Exec(`
		DELETE FROM meeting_access_control
		WHERE meeting_id = $1 AND user_id = $2
	`, meetingID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke meeting access: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user does not have explicit access to this meeting")
	}

	return nil
}

// ListMeetingAccessControl returns all users with explicit access to a meeting
// Does NOT include the meeting creator (who is owner by definition)
func ListMeetingAccessControl(meetingID string) ([]MeetingACLEntry, error) {
	query := `
		SELECT
			mac.id, mac.meeting_id, mac.user_id, mac.role,
			mac.granted_by, mac.granted_at, mac.updated_at,
			u.username, u.display_name
		FROM meeting_access_control mac
		JOIN users u ON mac.user_id = u.id
		WHERE mac.meeting_id = $1
		ORDER BY
			CASE mac.role
				WHEN 'owner' THEN 1
				WHEN 'editor' THEN 2
				WHEN 'viewer' THEN 3
			END,
			u.display_name ASC
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list meeting access control: %w", err)
	}
	defer rows.Close()

	var entries []MeetingACLEntry
	for rows.Next() {
		var entry MeetingACLEntry
		var grantedBy sql.NullInt64
		err := rows.Scan(
			&entry.ID,
			&entry.MeetingID,
			&entry.UserID,
			&entry.Role,
			&grantedBy,
			&entry.GrantedAt,
			&entry.UpdatedAt,
			&entry.Username,
			&entry.DisplayName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access control entry: %w", err)
		}

		if grantedBy.Valid {
			grantedByInt := int(grantedBy.Int64)
			entry.GrantedBy = &grantedByInt
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read access control entries: %w", err)
	}

	return entries, nil
}

// GetAvailableParticipants returns participants without explicit ACL entries
// Useful for autocomplete when granting access to new users
// Excludes the meeting creator (who is owner by definition)
func GetAvailableParticipants(meetingID string) ([]MeetingParticipant, error) {
	// Get meeting creator to exclude them
	var createdBy sql.NullInt64
	err := DB.QueryRow(`SELECT created_by FROM meetings WHERE id = $1`, meetingID).Scan(&createdBy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("meeting not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check meeting creator: %w", err)
	}

	query := `
		SELECT DISTINCT
			mp.id, mp.meeting_id, mp.user_id, mp.participant_name,
			mp.target_language, mp.joined_at, mp.left_at, mp.is_active
		FROM meeting_participants mp
		LEFT JOIN meeting_access_control mac
			ON mp.meeting_id = mac.meeting_id AND mp.user_id = mac.user_id
		WHERE mp.meeting_id = $1
			AND mp.user_id IS NOT NULL
			AND mac.id IS NULL
			AND ($2::INTEGER IS NULL OR mp.user_id != $2)
		ORDER BY mp.participant_name ASC
	`

	var createdByParam interface{}
	if createdBy.Valid {
		createdByInt := int(createdBy.Int64)
		createdByParam = &createdByInt
	}

	rows, err := DB.Query(query, meetingID, createdByParam)
	if err != nil {
		return nil, fmt.Errorf("failed to get available participants: %w", err)
	}
	defer rows.Close()

	var participants []MeetingParticipant
	for rows.Next() {
		var p MeetingParticipant
		var leftAt sql.NullTime
		err := rows.Scan(
			&p.ID,
			&p.MeetingID,
			&p.UserID,
			&p.ParticipantName,
			&p.TargetLanguage,
			&p.JoinedAt,
			&leftAt,
			&p.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}

		if leftAt.Valid {
			p.LeftAt = &leftAt.Time
		}

		participants = append(participants, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read available participants: %w", err)
	}

	return participants, nil
}

// AutoGrantViewerAccess automatically grants viewer access to a participant
// Should be called when a user joins a meeting
// Only grants access if user doesn't already have an ACL entry
func AutoGrantViewerAccess(meetingID string, userID int) error {
	// Check if user is the meeting creator (they're already owner)
	var createdBy sql.NullInt64
	err := DB.QueryRow(`SELECT created_by FROM meetings WHERE id = $1`, meetingID).Scan(&createdBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("meeting not found")
		}
		return fmt.Errorf("failed to check meeting creator: %w", err)
	}

	if createdBy.Valid && int(createdBy.Int64) == userID {
		return nil // Creator is owner by definition, no ACL entry needed
	}

	// Check if user already has an ACL entry
	var existingRole string
	err = DB.QueryRow(`
		SELECT role FROM meeting_access_control
		WHERE meeting_id = $1 AND user_id = $2
	`, meetingID, userID).Scan(&existingRole)

	if err == nil {
		return nil // User already has access, don't modify
	}

	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing access: %w", err)
	}

	// Grant viewer access (granted_by is NULL for auto-grants)
	query := `
		INSERT INTO meeting_access_control (meeting_id, user_id, role, granted_by, granted_at, updated_at)
		VALUES ($1, $2, $3, NULL, NOW(), NOW())
		ON CONFLICT (meeting_id, user_id) DO NOTHING
	`
	_, err = DB.Exec(query, meetingID, userID, RoleViewer)
	if err != nil {
		return fmt.Errorf("failed to auto-grant viewer access: %w", err)
	}

	return nil
}
