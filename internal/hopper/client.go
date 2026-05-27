package hopper

import (
	"context"
	"net/url"
	"strings"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"google.golang.org/api/option"
)

// Scheme returns the URI scheme (e.g. "spanner"), or "" if the URI is invalid.
func Scheme(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

// Client wraps the Spanner data and admin clients for a single database.
type Client struct {
	database string
	client   *spanner.Client
	admin    *database.DatabaseAdminClient
}

// NewClient connects to a Spanner database addressed by a
// spanner://projects/P/instances/I/databases/D[?credentials=/path/to/key.json] URI.
func NewClient(ctx context.Context, uri string) (*Client, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	db := u.Host + u.Path

	opts := []option.ClientOption{}
	if credentials := u.Query().Get("credentials"); credentials != "" {
		opts = append(opts, option.WithCredentialsFile(credentials))
	}
	client, err := spanner.NewClient(ctx, db, opts...)
	if err != nil {
		return nil, err
	}
	admin, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		client.Close()
		return nil, err
	}

	return &Client{
		database: db,
		client:   client,
		admin:    admin,
	}, nil
}

// Close releases the underlying Spanner clients.
func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
	if c.admin != nil {
		_ = c.admin.Close()
	}
}

// GetDatabaseDDL returns the database schema as a single DDL string,
// statements separated by ";\n".
func (c *Client) GetDatabaseDDL(ctx context.Context) (string, error) {
	resp, err := c.admin.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: c.database,
	})
	if err != nil {
		return "", err
	}
	return strings.Join(resp.Statements, ";\n"), nil
}

// Apply commits the given mutations to the database.
func (c *Client) Apply(ctx context.Context, ms []*spanner.Mutation) error {
	_, err := c.client.Apply(ctx, ms)
	return err
}
