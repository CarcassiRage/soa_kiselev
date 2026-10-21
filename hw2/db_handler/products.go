package db_handler

import (
	"context"
	"hw2/internal/api"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductsDB struct {
	pool *pgxpool.Pool
}

func NewProductsDB(pool *pgxpool.Pool) *ProductsDB {
	return &ProductsDB{pool: pool}
}

func (r *ProductsDB) Create(
	ctx context.Context,
	input api.ProductCreate,
) (api.ProductResponse, error) {
	var product api.ProductResponse
	err := r.pool.QueryRow(ctx, `
        INSERT INTO products (name, description, price, stock, category, status)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, name, description, price, stock, category, status, created_at, updated_at
    `, input.Name, input.Description, input.Price, input.Stock, input.Category, input.Status,
	).Scan(&product.Id, &product.Name, &product.Description, &product.Price, &product.Stock, &product.Category,
		&product.Status, &product.CreatedAt, &product.UpdatedAt)
	return product, err
}

func (r *ProductsDB) Get(ctx context.Context, id int64) (api.ProductResponse, error) {
	var product api.ProductResponse
	err := r.pool.QueryRow(ctx, `
        SELECT id, name, description, price, stock, category, status, created_at, updated_at FROM products WHERE id = $1
    `, id).Scan(&product.Id, &product.Name, &product.Description, &product.Price, &product.Stock, &product.Category,
		&product.Status, &product.CreatedAt, &product.UpdatedAt)
	return product, err
}

func (r *ProductsDB) List(ctx context.Context, page, size int, status *api.ProductStatus, category *string) (api.ProductPage, error) {
	result := api.ProductPage{
		Items: []api.ProductResponse{},
		Page:  page,
		Size:  size,
	}
	var total int64
	err := r.pool.QueryRow(ctx, `
        SELECT COUNT(*) FROM products WHERE ($1::text IS NULL OR status = $1) AND ($2::text IS NULL OR category = $2)
    `, status, category).Scan(&total)
	if err != nil {
		return result, err
	}
	result.TotalElements = total
	rows, err := r.pool.Query(ctx, `
        SELECT id, name, description, price, stock, category, status, created_at, updated_at
        FROM products WHERE ($1::text IS NULL OR status = $1) AND ($2::text IS NULL OR category = $2)
		ORDER BY id LIMIT $3 OFFSET $4
    `, status, category, size, page*size)
	if err != nil {
		return result, err
	}

	defer rows.Close()
	for rows.Next() {
		var product api.ProductResponse
		err := rows.Scan(&product.Id, &product.Name, &product.Description, &product.Price, &product.Stock,
			&product.Category, &product.Status, &product.CreatedAt, &product.UpdatedAt)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, product)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func (r *ProductsDB) Update(ctx context.Context, id int64, input api.ProductUpdate) (api.ProductResponse, error) {
	var product api.ProductResponse
	err := r.pool.QueryRow(ctx, `
        UPDATE products SET name = $2, description = $3, price = $4, stock = $5, category = $6, status = $7
        WHERE id = $1 RETURNING id, name, description, price, stock, category, status, created_at, updated_at
    `, id, input.Name, input.Description, input.Price, input.Stock, input.Category, input.Status,
	).Scan(&product.Id, &product.Name, &product.Description, &product.Price, &product.Stock, &product.Category,
		&product.Status, &product.CreatedAt, &product.UpdatedAt)
	return product, err
}

func (r *ProductsDB) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE products SET status = 'ARCHIVED' WHERE id = $1
    `, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
