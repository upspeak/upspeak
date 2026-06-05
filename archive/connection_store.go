package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveConnection persists a connection, encrypting its credentials at rest.
// Version == 0 creates (generates CONN short ID); Version > 0 updates with an
// optimistic concurrency check.
func (a *LocalArchive) saveConnection(conn *core.Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	configJSON, err := json.Marshal(conn.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal connection config: %w", err)
	}

	credsBlob, err := a.encryptCredentials(conn.Credentials)
	if err != nil {
		return err
	}

	var rateLimitJSON []byte
	if conn.RateLimit != nil {
		rateLimitJSON, err = json.Marshal(conn.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal connection rate limit: %w", err)
		}
	}

	now := time.Now().UTC()

	if conn.Version == 0 {
		seq, err := nextUserSequence(a.db, conn.OwnerID, "connection")
		if err != nil {
			return fmt.Errorf("failed to generate connection short ID: %w", err)
		}
		conn.ShortID = core.FormatShortID(core.PrefixConnection, seq)
		conn.Version = 1
		conn.CreatedAt = now
		conn.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO connections (id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, conn.ID.String(), conn.ShortID, conn.OwnerID.String(), conn.Name,
			string(conn.Connector), string(conn.AuthType), string(configJSON), credsBlob,
			string(conn.Status), stringOrNil(rateLimitJSON), timePtrString(conn.CredentialExpiry),
			timePtrString(conn.LastCheckedAt), strPtrAny(conn.LastError), conn.Version,
			conn.CreatedBy.String(), conn.CreatedAt.Format(time.RFC3339), conn.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert connection: %w", err)
		}
		return nil
	}

	conn.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE connections
		SET name = ?, connector = ?, auth_type = ?, config = ?, credentials = ?, status = ?, rate_limit = ?, credential_expiry = ?, last_checked_at = ?, last_error = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, conn.Name, string(conn.Connector), string(conn.AuthType), string(configJSON), credsBlob,
		string(conn.Status), stringOrNil(rateLimitJSON), timePtrString(conn.CredentialExpiry),
		timePtrString(conn.LastCheckedAt), strPtrAny(conn.LastError),
		conn.UpdatedAt.Format(time.RFC3339), conn.ID.String(), conn.Version)
	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{EntityType: "connection", EntityID: conn.ID, Expected: conn.Version}
	}
	conn.Version++
	return nil
}

// encryptCredentials marshals and encrypts credentials. Empty credentials store
// as NULL; non-empty credentials without a configured cipher fail closed.
func (a *LocalArchive) encryptCredentials(creds map[string]any) (any, error) {
	if len(creds) == 0 {
		return nil, nil
	}
	if a.cipher == nil {
		return nil, core.ErrSecretKeyMissing
	}
	plain, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}
	ct, err := a.cipher.Encrypt(plain)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}
	return ct, nil
}

// getConnection retrieves a connection by UUID, decrypting credentials.
func (a *LocalArchive) getConnection(connID uuid.UUID) (*core.Connection, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at
		FROM connections WHERE id = ?
	`, connID.String())
	return a.scanConnection(row)
}

// listConnections returns paginated connections for an owner.
func (a *LocalArchive) listConnections(ownerID uuid.UUID, opts core.ConnectionListOptions) ([]core.Connection, int, error) {
	where := `WHERE owner_id = ?`
	args := []any{ownerID.String()}
	if opts.Connector != "" {
		where += ` AND connector = ?`
		args = append(args, string(opts.Connector))
	}
	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, string(opts.Status))
	}

	var total int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM connections `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count connections: %w", err)
	}

	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "name":
		sortBy = opts.SortBy
	}
	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at
		FROM connections %s ORDER BY %s %s LIMIT ? OFFSET ?`, where, sortBy, order)
	rows, err := a.db.Query(query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	var conns []core.Connection
	for rows.Next() {
		c, err := a.scanConnection(rows)
		if err != nil {
			return nil, 0, err
		}
		conns = append(conns, *c)
	}
	return conns, total, nil
}

// deleteConnection deletes a connection by UUID.
func (a *LocalArchive) deleteConnection(connID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM connections WHERE id = ?`, connID.String())
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("connection", connID.String())
	}
	return nil
}

// getConnectionReferences returns sources and sinks referencing a connection,
// used to block deletion of an in-use connection.
func (a *LocalArchive) getConnectionReferences(connID uuid.UUID) ([]core.FilterReference, error) {
	var refs []core.FilterReference
	for _, q := range []struct{ table, kind string }{{"sources", "source"}, {"sinks", "sink"}} {
		rows, err := a.db.Query(
			fmt.Sprintf(`SELECT id, name FROM %s WHERE connection_id = ?`, q.table), connID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to query %s references: %w", q.table, err)
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan %s reference: %w", q.table, err)
			}
			refs = append(refs, core.FilterReference{EntityType: q.kind, EntityID: id, EntityName: name})
		}
		rows.Close()
	}
	return refs, nil
}

// rowScanner abstracts *sql.Row and *sql.Rows for a shared scan helper.
type rowScanner interface{ Scan(dest ...any) error }

func (a *LocalArchive) scanConnection(s rowScanner) (*core.Connection, error) {
	var conn core.Connection
	var idStr, ownerStr, connectorStr, authStr, configStr, statusStr, createdByStr, createdAt, updatedAt string
	var credsBlob []byte
	var rateLimitStr, credExpiry, lastChecked, lastErr sql.NullString

	err := s.Scan(&idStr, &conn.ShortID, &ownerStr, &conn.Name, &connectorStr, &authStr,
		&configStr, &credsBlob, &statusStr, &rateLimitStr, &credExpiry, &lastChecked, &lastErr,
		&conn.Version, &createdByStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("connection", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan connection: %w", err)
	}

	if conn.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection ID: %w", err)
	}
	if conn.OwnerID, err = uuid.Parse(ownerStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection owner_id: %w", err)
	}
	if conn.CreatedBy, err = uuid.Parse(createdByStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection created_by: %w", err)
	}
	conn.Connector = core.ConnectorType(connectorStr)
	conn.AuthType = core.AuthType(authStr)
	conn.Status = core.ResourceStatus(statusStr)

	if configStr != "" {
		if err := json.Unmarshal([]byte(configStr), &conn.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection config: %w", err)
		}
	}
	if len(credsBlob) > 0 {
		if a.cipher == nil {
			return nil, core.ErrSecretKeyMissing
		}
		plain, err := a.cipher.Decrypt(credsBlob)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
		}
		if err := json.Unmarshal(plain, &conn.Credentials); err != nil {
			return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
		}
	}
	if rateLimitStr.Valid && rateLimitStr.String != "" {
		var rl core.RateLimit
		if err := json.Unmarshal([]byte(rateLimitStr.String), &rl); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection rate limit: %w", err)
		}
		conn.RateLimit = &rl
	}
	if conn.CredentialExpiry, err = parseNullableTime(credExpiry); err != nil {
		return nil, err
	}
	if conn.LastCheckedAt, err = parseNullableTime(lastChecked); err != nil {
		return nil, err
	}
	if lastErr.Valid && lastErr.String != "" {
		s := lastErr.String
		conn.LastError = &s
	}
	if conn.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("failed to parse connection created_at: %w", err)
	}
	if conn.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return nil, fmt.Errorf("failed to parse connection updated_at: %w", err)
	}
	return &conn, nil
}

// timePtrString formats a *time.Time as RFC3339 for SQL, or nil.
func timePtrString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// parseNullableTime parses an optional RFC3339 timestamp column.
func parseNullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	return &t, nil
}
