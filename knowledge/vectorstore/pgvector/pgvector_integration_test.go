//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package pgvector

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore"
)

// mockPool wraps pgxmock to implement the required interface
type mockPoolWrapper struct {
	mock pgxmock.PgxPoolIface
}

func (m *mockPoolWrapper) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return m.mock.Exec(ctx, sql, arguments...)
}

func (m *mockPoolWrapper) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return m.mock.Query(ctx, sql, args...)
}

func (m *mockPoolWrapper) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.mock.QueryRow(ctx, sql, args...)
}

func (m *mockPoolWrapper) Close() {
	m.mock.Close()
}

// TestVectorStore_Add_WithMock tests Add with mocked database
func TestVectorStore_Add_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	t.Run("nil_document", func(t *testing.T) {
		err := vs.Add(context.Background(), nil, []float64{0.1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document is required")
	})

	t.Run("empty_id", func(t *testing.T) {
		doc := &document.Document{ID: "", Content: "test"}
		err := vs.Add(context.Background(), doc, []float64{0.1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document ID is required")
	})

	t.Run("empty_embedding", func(t *testing.T) {
		doc := &document.Document{ID: "doc1", Content: "test"}
		err := vs.Add(context.Background(), doc, []float64{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector embedding is required")
	})

	t.Run("dimension_mismatch", func(t *testing.T) {
		doc := &document.Document{ID: "doc1", Content: "test"}
		err := vs.Add(context.Background(), doc, []float64{0.1, 0.2})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector embedding dimension mismatch")
	})

	t.Run("successful_add", func(t *testing.T) {
		doc := &document.Document{
			ID:       "doc1",
			Name:     "Test",
			Content:  "Content",
			Metadata: map[string]any{"key": "value"},
		}
		embedding := make([]float64, 1536)

		mock.ExpectExec("INSERT INTO").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err := vs.Add(context.Background(), doc, embedding)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database_error", func(t *testing.T) {
		doc := &document.Document{ID: "doc1", Content: "test"}
		embedding := make([]float64, 1536)

		mock.ExpectExec("INSERT INTO").
			WillReturnError(errors.New("db error"))

		err := vs.Add(context.Background(), doc, embedding)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector insert document")
	})
}

// TestVectorStore_Get_WithMock tests Get with mocked database
func TestVectorStore_Get_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	t.Run("empty_id", func(t *testing.T) {
		_, _, err := vs.Get(context.Background(), "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector id is required")
	})

	t.Run("successful_get", func(t *testing.T) {
		rows := mock.NewRows([]string{"id", "name", "content", "embedding", "metadata", "created_at", "updated_at", "score"}).
			AddRow("doc1", "Test", "Content", pgvector.NewVector([]float32{0.1, 0.2}), `{"key":"value"}`, int64(1234567890), int64(1234567891), 0.95)

		mock.ExpectQuery("SELECT").
			WithArgs("doc1").
			WillReturnRows(rows)

		doc, emb, err := vs.Get(context.Background(), "doc1")
		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, "doc1", doc.ID)
		assert.NotEmpty(t, emb)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not_found", func(t *testing.T) {
		mock.ExpectQuery("SELECT").
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, _, err := vs.Get(context.Background(), "missing")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector get document")
	})
}

// TestVectorStore_Delete_WithMock tests Delete with mocked database
func TestVectorStore_Delete_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	t.Run("empty_id", func(t *testing.T) {
		err := vs.Delete(context.Background(), "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector id is required")
	})

	t.Run("successful_delete", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM").
			WithArgs("doc1").
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		err := vs.Delete(context.Background(), "doc1")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not_found", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM").
			WithArgs("missing").
			WillReturnResult(pgxmock.NewResult("DELETE", 0))

		err := vs.Delete(context.Background(), "missing")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document not found")
	})

	t.Run("database_error", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM").
			WillReturnError(errors.New("db error"))

		err := vs.Delete(context.Background(), "doc1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector delete document")
	})
}

// TestVectorStore_Update_WithMock tests Update with mocked database
func TestVectorStore_Update_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	t.Run("nil_document", func(t *testing.T) {
		err := vs.Update(context.Background(), nil, []float64{0.1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document is required")
	})

	t.Run("empty_id", func(t *testing.T) {
		doc := &document.Document{ID: ""}
		err := vs.Update(context.Background(), doc, []float64{0.1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document ID is required")
	})

	t.Run("document_not_exists", func(t *testing.T) {
		doc := &document.Document{ID: "missing", Content: "test"}
		embedding := make([]float64, 1536)

		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		err := vs.Update(context.Background(), doc, embedding)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector check document existence")
	})

	t.Run("dimension_mismatch", func(t *testing.T) {
		doc := &document.Document{ID: "doc1", Content: "test"}

		rows := mock.NewRows([]string{"exists"}).AddRow(1)
		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("doc1").
			WillReturnRows(rows)

		err := vs.Update(context.Background(), doc, []float64{0.1, 0.2})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector embedding dimension mismatch")
	})

	t.Run("successful_update", func(t *testing.T) {
		doc := &document.Document{
			ID:       "doc1",
			Name:     "Updated",
			Content:  "Updated Content",
			Metadata: map[string]any{"updated": true},
		}
		embedding := make([]float64, 1536)

		rows := mock.NewRows([]string{"exists"}).AddRow(1)
		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("doc1").
			WillReturnRows(rows)

		mock.ExpectExec("UPDATE").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err := vs.Update(context.Background(), doc, embedding)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		doc := &document.Document{ID: "doc1", Content: "test"}
		embedding := make([]float64, 1536)

		rows := mock.NewRows([]string{"exists"}).AddRow(1)
		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("doc1").
			WillReturnRows(rows)

		mock.ExpectExec("UPDATE").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := vs.Update(context.Background(), doc, embedding)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector document not updated")
	})
}

// TestVectorStore_Search_WithMock tests Search validation with mocked database
func TestVectorStore_Search_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:            wrapper,
		option:          defaultOptions,
		filterConverter: &pgVectorConverter{},
	}

	t.Run("nil_query", func(t *testing.T) {
		_, err := vs.Search(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector: query is required")
	})

	t.Run("invalid_search_mode", func(t *testing.T) {
		query := &vectorstore.SearchQuery{SearchMode: 999}
		_, err := vs.Search(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector: invalid search mode")
	})

	t.Run("vector_search_empty_vector", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeVector,
			Vector:     []float64{},
		}
		_, err := vs.searchByVector(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "searching with a nil or empty vector")
	})

	t.Run("vector_dimension_mismatch", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeVector,
			Vector:     []float64{0.1, 0.2},
		}
		_, err := vs.searchByVector(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector vector dimension mismatch")
	})

	t.Run("keyword_search_empty_query", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeKeyword,
			Query:      "",
		}
		_, err := vs.searchByKeyword(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector keyword is required")
	})

	t.Run("hybrid_search_empty_vector", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeHybrid,
			Vector:     []float64{},
		}
		_, err := vs.searchByHybrid(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector vector is required for hybrid search")
	})

	t.Run("hybrid_search_dimension_mismatch", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeHybrid,
			Vector:     []float64{0.1, 0.2},
			Query:      "test",
		}
		_, err := vs.searchByHybrid(context.Background(), query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector vector dimension mismatch")
	})

	t.Run("successful_vector_search", func(t *testing.T) {
		query := &vectorstore.SearchQuery{
			SearchMode: vectorstore.SearchModeVector,
			Vector:     make([]float64, 1536),
			Limit:      10,
		}

		rows := mock.NewRows([]string{"id", "name", "content", "embedding", "metadata", "created_at", "updated_at", "score"}).
			AddRow("doc1", "Test", "Content", pgvector.NewVector(make([]float32, 1536)), `{}`, int64(1234567890), int64(1234567891), 0.95)

		mock.ExpectQuery("SELECT").
			WillReturnRows(rows)

		result, err := vs.searchByVector(context.Background(), query)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Results, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestVectorStore_Count_WithMock tests Count with mocked database
func TestVectorStore_Count_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:            wrapper,
		option:          defaultOptions,
		filterConverter: &pgVectorConverter{},
	}

	t.Run("successful_count", func(t *testing.T) {
		rows := mock.NewRows([]string{"count"}).AddRow(42)
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(rows)

		count, err := vs.Count(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 42, count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database_error", func(t *testing.T) {
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(errors.New("db error"))

		_, err := vs.Count(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pgvector count documents")
	})
}

// TestVectorStore_DeleteByFilter_WithMock tests DeleteByFilter with mocked database
func TestVectorStore_DeleteByFilter_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:            wrapper,
		option:          defaultOptions,
		filterConverter: &pgVectorConverter{},
	}

	t.Run("delete_all", func(t *testing.T) {
		mock.ExpectExec("TRUNCATE TABLE").
			WillReturnResult(pgxmock.NewResult("TRUNCATE", 0))

		err := vs.DeleteByFilter(context.Background(), vectorstore.WithDeleteAll(true))
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("delete_by_ids", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM").
			WillReturnResult(pgxmock.NewResult("DELETE", 2))

		err := vs.DeleteByFilter(context.Background(),
			vectorstore.WithDeleteDocumentIDs([]string{"doc1", "doc2"}))
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid_config_delete_all_with_ids", func(t *testing.T) {
		err := vs.DeleteByFilter(context.Background(),
			vectorstore.WithDeleteAll(true),
			vectorstore.WithDeleteDocumentIDs([]string{"doc1"}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delete all documents, but document ids or filter are provided")
	})

	t.Run("no_filter_conditions", func(t *testing.T) {
		err := vs.DeleteByFilter(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no filter conditions specified")
	})
}

// TestVectorStore_GetMetadata_WithMock tests GetMetadata with mocked database
func TestVectorStore_GetMetadata_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:            wrapper,
		option:          defaultOptions,
		filterConverter: &pgVectorConverter{},
	}

	t.Run("successful_get_metadata", func(t *testing.T) {
		rows := mock.NewRows([]string{"id", "name", "content", "embedding", "metadata", "created_at", "updated_at", "score"}).
			AddRow("doc1", "Test", "Content", pgvector.NewVector([]float32{0.1}), `{"key":"value"}`, int64(1234567890), int64(1234567891), 0.0)

		mock.ExpectQuery("SELECT").WillReturnRows(rows)

		result, err := vs.GetMetadata(context.Background(), vectorstore.WithGetMetadataLimit(10), vectorstore.WithGetMetadataOffset(0))
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("get_all_metadata", func(t *testing.T) {
		// First batch
		rows1 := mock.NewRows([]string{"id", "name", "content", "embedding", "metadata", "created_at", "updated_at", "score"})
		for i := 0; i < 5000; i++ {
			rows1.AddRow("doc1", "Test", "Content", pgvector.NewVector([]float32{0.1}), `{}`, int64(1234567890), int64(1234567891), 0.0)
		}
		mock.ExpectQuery("SELECT").WillReturnRows(rows1)

		// Second batch (less than 5000, indicating end)
		rows2 := mock.NewRows([]string{"id", "name", "content", "embedding", "metadata", "created_at", "updated_at", "score"}).
			AddRow("doc2", "Test", "Content", pgvector.NewVector([]float32{0.1}), `{}`, int64(1234567890), int64(1234567891), 0.0)
		mock.ExpectQuery("SELECT").WillReturnRows(rows2)

		result, err := vs.GetMetadata(context.Background(), vectorstore.WithGetMetadataLimit(-1), vectorstore.WithGetMetadataOffset(-1))
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestVectorStore_Close tests Close method
func TestVectorStore_Close(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	err = vs.Close()
	assert.NoError(t, err)
}

// TestVectorStore_documentExists_WithMock tests documentExists with mocked database
func TestVectorStore_documentExists_WithMock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	wrapper := &mockPoolWrapper{mock: mock}
	vs := &VectorStore{
		pool:   wrapper,
		option: defaultOptions,
	}

	t.Run("document_exists", func(t *testing.T) {
		rows := mock.NewRows([]string{"exists"}).AddRow(1)
		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("doc1").
			WillReturnRows(rows)

		exists, err := vs.documentExists(context.Background(), "doc1")
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("document_not_exists", func(t *testing.T) {
		mock.ExpectQuery("SELECT 1 FROM").
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		exists, err := vs.documentExists(context.Background(), "missing")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("database_error", func(t *testing.T) {
		mock.ExpectQuery("SELECT 1 FROM").
			WillReturnError(errors.New("connection error"))

		_, err := vs.documentExists(context.Background(), "doc1")
		assert.Error(t, err)
	})
}

// TestVectorStore_buildQueryFilter tests buildQueryFilter
func TestVectorStore_buildQueryFilter(t *testing.T) {
	vs := &VectorStore{
		option:          defaultOptions,
		filterConverter: &pgVectorConverter{},
	}

	t.Run("nil_condition", func(t *testing.T) {
		qb := newVectorQueryBuilder(defaultOptions)
		err := vs.buildQueryFilter(qb, nil)
		assert.NoError(t, err)
	})

	t.Run("with_ids", func(t *testing.T) {
		qb := newVectorQueryBuilder(defaultOptions)
		filter := &vectorstore.SearchFilter{
			IDs: []string{"doc1", "doc2"},
		}
		err := vs.buildQueryFilter(qb, filter)
		assert.NoError(t, err)
	})

	t.Run("with_metadata", func(t *testing.T) {
		qb := newVectorQueryBuilder(defaultOptions)
		filter := &vectorstore.SearchFilter{
			Metadata: map[string]any{"category": "test"},
		}
		err := vs.buildQueryFilter(qb, filter)
		assert.NoError(t, err)
	})

	t.Run("with_filter_condition", func(t *testing.T) {
		qb := newVectorQueryBuilder(defaultOptions)
		filter := &vectorstore.SearchFilter{
			FilterCondition: &vectorstore.UniversalFilterCondition{
				Field:    "age",
				Operator: "eq",
				Value:    25,
			},
		}
		err := vs.buildQueryFilter(qb, filter)
		assert.NoError(t, err)
	})
}
