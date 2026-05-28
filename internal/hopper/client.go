package hopper

import (
	"context"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
)

const DefaultClearBatchSize = 100

func Scheme(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

type Client struct {
	database string
	client   *spanner.Client
	admin    *database.DatabaseAdminClient
}

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

func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
	if c.admin != nil {
		_ = c.admin.Close()
	}
}

func (c *Client) GetDatabaseDDL(ctx context.Context) (string, error) {
	resp, err := c.admin.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: c.database,
	})
	if err != nil {
		return "", err
	}
	return strings.Join(resp.Statements, ";\n"), nil
}

func (c *Client) Apply(ctx context.Context, ms []*spanner.Mutation) error {
	_, err := c.client.Apply(ctx, ms)
	return err
}

func (c *Client) Clear(ctx context.Context, t *Table, batch int, onProgress func(deleted int64)) error {
	if batch <= 0 {
		batch = DefaultClearBatchSize
	}
	var deleted int64
	for {
		keys, err := c.readPKBatch(ctx, t, batch)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		ms := []*spanner.Mutation{spanner.Delete(t.Name, spanner.KeySetFromKeys(keys...))}
		if _, err := c.client.Apply(ctx, ms); err != nil {
			if isTooManyMutations(err) && batch > 1 {
				batch = max(batch/2, 1)
				continue
			}
			return err
		}
		deleted += int64(len(keys))
		if onProgress != nil {
			onProgress(deleted)
		}
	}
}

func isTooManyMutations(err error) bool {
	return spanner.ErrCode(err) == codes.InvalidArgument && strings.Contains(err.Error(), "too many mutations")
}

func (c *Client) readPKBatch(ctx context.Context, t *Table, limit int) ([]spanner.Key, error) {
	cols := make([]string, len(t.PrimaryKeys))
	for i, p := range t.PrimaryKeys {
		cols[i] = "`" + p + "`"
	}
	stmt := spanner.Statement{
		SQL: fmt.Sprintf("SELECT %s FROM `%s` LIMIT %d", strings.Join(cols, ","), t.Name, limit),
	}
	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()
	var keys []spanner.Key
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		k, err := decodePK(row, t)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func decodePK(row *spanner.Row, t *Table) (spanner.Key, error) {
	k := make(spanner.Key, len(t.PrimaryKeys))
	for i, pkName := range t.PrimaryKeys {
		col, ok := t.Column(pkName)
		if !ok {
			return nil, fmt.Errorf("primary key column %q not found", pkName)
		}
		v, err := decodeKeyValue(row, i, col.Type.Base)
		if err != nil {
			return nil, err
		}
		k[i] = v
	}
	return k, nil
}

func decodeKeyValue(row *spanner.Row, idx int, base ast.ScalarTypeName) (any, error) {
	switch base {
	case ast.Int64TypeName:
		var v int64
		err := row.Column(idx, &v)
		return v, err
	case ast.StringTypeName:
		var v string
		err := row.Column(idx, &v)
		return v, err
	case ast.BytesTypeName:
		var v []byte
		err := row.Column(idx, &v)
		return v, err
	case ast.BoolTypeName:
		var v bool
		err := row.Column(idx, &v)
		return v, err
	case ast.Float64TypeName:
		var v float64
		err := row.Column(idx, &v)
		return v, err
	case ast.Float32TypeName:
		var v float32
		err := row.Column(idx, &v)
		return v, err
	case ast.TimestampTypeName:
		var v time.Time
		err := row.Column(idx, &v)
		return v, err
	case ast.DateTypeName:
		var v civil.Date
		err := row.Column(idx, &v)
		return v, err
	case ast.NumericTypeName:
		var v big.Rat
		err := row.Column(idx, &v)
		return v, err
	default:
		return nil, fmt.Errorf("unsupported primary key type: %v", base)
	}
}
