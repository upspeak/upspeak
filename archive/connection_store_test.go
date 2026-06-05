package archive

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/secrets"
)

func withCipher(t *testing.T, a *LocalArchive) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	c, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	a.SetSecretCipher(c)
}

func sampleConnection(ownerID uuid.UUID) *core.Connection {
	return &core.Connection{
		ID:          core.NewID(),
		OwnerID:     ownerID,
		Name:        "my-discourse",
		Connector:   core.ConnectorDiscourse,
		AuthType:    core.AuthAPIKey,
		Config:      map[string]any{"base_url": "https://forum.example"},
		Credentials: map[string]any{"api_key": "super-secret"},
		Status:      core.StatusActive,
		CreatedBy:   ownerID,
	}
}

func TestSaveConnection_EncryptsCredentials(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	owner := core.NewID()

	conn := sampleConnection(owner)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	if conn.ShortID == "" {
		t.Fatal("expected a CONN short ID")
	}

	var raw []byte
	if err := a.db.QueryRow(`SELECT credentials FROM connections WHERE id = ?`, conn.ID.String()).Scan(&raw); err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("credentials column is empty")
	}
	if contains(raw, "super-secret") {
		t.Fatal("credentials stored in plaintext")
	}

	got, err := a.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if got.Credentials["api_key"] != "super-secret" {
		t.Fatalf("credentials not round-tripped: %v", got.Credentials)
	}
}

func TestSaveConnection_FailsClosedWithoutCipher(t *testing.T) {
	a := setupTestArchive(t)
	owner := core.NewID()
	if err := a.SaveConnection(sampleConnection(owner)); !errors.Is(err, core.ErrSecretKeyMissing) {
		t.Fatalf("expected ErrSecretKeyMissing, got %v", err)
	}
}

func TestGetConnectionReferences(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	repo := createTestRepo(t, a)

	conn := sampleConnection(repo.OwnerID)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	src := &core.Source{
		ID: core.NewID(), RepoID: repo.ID, Name: "s", Connector: core.ConnectorDiscourse,
		ConnectionID: &conn.ID, CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	refs, err := a.GetConnectionReferences(conn.ID)
	if err != nil {
		t.Fatalf("GetConnectionReferences: %v", err)
	}
	if len(refs) != 1 || refs[0].EntityType != "source" {
		t.Fatalf("expected one source reference, got %v", refs)
	}
}

// TestGetConnection_FailsClosedOnRead locks in the read-side fail-closed
// guarantee: a connection whose credentials blob is present cannot be read back
// without a cipher configured.
func TestGetConnection_FailsClosedOnRead(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	owner := core.NewID()

	conn := sampleConnection(owner)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	// Remove the cipher, then attempt to read the encrypted connection.
	a.SetSecretCipher(nil)
	if _, err := a.GetConnection(conn.ID); !errors.Is(err, core.ErrSecretKeyMissing) {
		t.Fatalf("expected ErrSecretKeyMissing on read without cipher, got %v", err)
	}
}

// TestGetConnection_TamperDetected locks in the GCM integrity guarantee: a
// corrupted credentials blob surfaces as a decrypt error rather than silent or
// partial credentials.
func TestGetConnection_TamperDetected(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	owner := core.NewID()

	conn := sampleConnection(owner)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	// Corrupt the stored ciphertext.
	var raw []byte
	if err := a.db.QueryRow(`SELECT credentials FROM connections WHERE id = ?`, conn.ID.String()).Scan(&raw); err != nil {
		t.Fatalf("read raw: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF
	if _, err := a.db.Exec(`UPDATE connections SET credentials = ? WHERE id = ?`, raw, conn.ID.String()); err != nil {
		t.Fatalf("corrupt update: %v", err)
	}

	got, err := a.GetConnection(conn.ID)
	if err == nil {
		t.Fatalf("expected a decrypt error on tampered ciphertext, got connection %+v", got)
	}
}

func contains(b []byte, s string) bool {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}
