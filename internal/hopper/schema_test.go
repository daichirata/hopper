package hopper

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
)

const testDDL = `
CREATE TABLE Users (
  UserId STRING(36) NOT NULL,
  Name STRING(MAX),
  ShardId INT64 NOT NULL,
  CreatedAt TIMESTAMP NOT NULL,
  FullName STRING(MAX) AS (Name) STORED,
) PRIMARY KEY (UserId);

CREATE TABLE UserAvatars (
  UserId STRING(36) NOT NULL,
  AvatarId INT64 NOT NULL,
  Url STRING(MAX),
) PRIMARY KEY (UserId, AvatarId),
  INTERLEAVE IN PARENT Users ON DELETE CASCADE;
`

func TestParseSchema(t *testing.T) {
	s, err := ParseSchema("test", testDDL)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}

	users, ok := s.Table("Users")
	if !ok {
		t.Fatal("Users table not found")
	}
	if users.Parent != "" {
		t.Errorf("Users.Parent = %q, want empty", users.Parent)
	}
	if len(users.PrimaryKeys) != 1 || users.PrimaryKeys[0] != "UserId" {
		t.Errorf("Users.PrimaryKeys = %v, want [UserId]", users.PrimaryKeys)
	}

	uid, ok := users.Column("UserId")
	if !ok {
		t.Fatal("UserId column not found")
	}
	if uid.Type.Base != ast.StringTypeName || uid.Type.Size != 36 {
		t.Errorf("UserId type = %+v, want STRING size 36", uid.Type)
	}
	if !uid.NotNull {
		t.Error("UserId should be NOT NULL")
	}

	full, ok := users.Column("FullName")
	if !ok {
		t.Fatal("FullName column not found")
	}
	if !full.Generated {
		t.Error("FullName should be flagged as generated")
	}

	av, ok := s.Table("UserAvatars")
	if !ok {
		t.Fatal("UserAvatars table not found")
	}
	if av.Parent != "Users" {
		t.Errorf("UserAvatars.Parent = %q, want Users", av.Parent)
	}
	if len(av.PrimaryKeys) != 2 {
		t.Errorf("UserAvatars.PrimaryKeys = %v, want 2 keys", av.PrimaryKeys)
	}
	if !av.IsPrimaryKey("UserId") {
		t.Error("UserId should be part of UserAvatars PK")
	}
}
