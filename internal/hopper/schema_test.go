package hopper

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
)

const testDDL = `
CREATE TABLE Singers (
  SingerId   INT64 NOT NULL,
  FirstName  STRING(1024),
  LastName   STRING(1024),
  SingerInfo BYTES(MAX),
  FullName   STRING(MAX) AS (COALESCE(FirstName, '') || ' ' || COALESCE(LastName, '')) STORED,
) PRIMARY KEY (SingerId);

CREATE TABLE Albums (
  SingerId        INT64 NOT NULL,
  AlbumId         INT64 NOT NULL,
  AlbumTitle      STRING(MAX),
  MarketingBudget INT64,
) PRIMARY KEY (SingerId, AlbumId),
  INTERLEAVE IN PARENT Singers ON DELETE CASCADE;

CREATE TABLE Songs (
  SingerId INT64 NOT NULL,
  AlbumId  INT64 NOT NULL,
  TrackId  INT64 NOT NULL,
  SongName STRING(MAX),
) PRIMARY KEY (SingerId, AlbumId, TrackId),
  INTERLEAVE IN PARENT Albums ON DELETE CASCADE;

CREATE TABLE Concerts (
  ConcertId INT64 NOT NULL,
  SingerId  INT64 NOT NULL,
  Venue     STRING(MAX),
  CONSTRAINT FK_ConcertSinger FOREIGN KEY (SingerId) REFERENCES Singers (SingerId),
) PRIMARY KEY (ConcertId);
`

func TestParseSchema(t *testing.T) {
	s, err := ParseSchema("test", testDDL)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}

	singers, ok := s.Table("Singers")
	if !ok {
		t.Fatal("Singers table not found")
	}
	if singers.Parent != "" {
		t.Errorf("Singers.Parent = %q, want empty", singers.Parent)
	}
	if len(singers.PrimaryKeys) != 1 || singers.PrimaryKeys[0] != "SingerId" {
		t.Errorf("Singers.PrimaryKeys = %v, want [SingerId]", singers.PrimaryKeys)
	}

	sid, ok := singers.Column("SingerId")
	if !ok {
		t.Fatal("SingerId column not found")
	}
	if sid.Type.Base != ast.Int64TypeName {
		t.Errorf("SingerId type = %+v, want INT64", sid.Type)
	}
	if !sid.NotNull {
		t.Error("SingerId should be NOT NULL")
	}

	full, ok := singers.Column("FullName")
	if !ok {
		t.Fatal("FullName column not found")
	}
	if !full.Generated {
		t.Error("FullName should be flagged as generated")
	}

	albums, ok := s.Table("Albums")
	if !ok {
		t.Fatal("Albums table not found")
	}
	if albums.Parent != "Singers" {
		t.Errorf("Albums.Parent = %q, want Singers", albums.Parent)
	}
	if len(albums.PrimaryKeys) != 2 {
		t.Errorf("Albums.PrimaryKeys = %v, want 2 keys", albums.PrimaryKeys)
	}
	if !albums.IsPrimaryKey("SingerId") {
		t.Error("SingerId should be part of Albums PK")
	}

	songs, ok := s.Table("Songs")
	if !ok {
		t.Fatal("Songs table not found")
	}
	if songs.Parent != "Albums" {
		t.Errorf("Songs.Parent = %q, want Albums", songs.Parent)
	}

	concerts, ok := s.Table("Concerts")
	if !ok {
		t.Fatal("Concerts table not found")
	}
	if len(concerts.ForeignKeys) != 1 {
		t.Fatalf("Concerts.ForeignKeys = %d, want 1", len(concerts.ForeignKeys))
	}
	fk := concerts.ForeignKeys[0]
	if fk.RefTable != "Singers" || len(fk.Columns) != 1 || fk.Columns[0] != "SingerId" || fk.RefColumns[0] != "SingerId" {
		t.Errorf("Concerts FK = %+v, want SingerId -> Singers.SingerId", fk)
	}
}
