package main

import (
	"context"
	"hw2/db_handler"
	"hw2/handler"
	"hw2/internal/api"
	"hw2/middleware"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	db := os.Getenv("DB_URL")
	if db == "" {
		log.Fatal("DB_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	productsDB := db_handler.NewProductsDB(pool)
	handler := handler.NewHandler(*productsDB)
	r := gin.Default()
	r.Use(middleware.OpenAPIValidation())
	api.RegisterHandlersWithOptions(r, handler, api.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			details := map[string]interface{}{
				"parameter": err.Error(),
			}

			c.JSON(statusCode, api.ErrorResponse{
				ErrorCode: "VALIDATION_ERROR",
				Message:   "Invalid parameter",
				Details:   &details,
			})
		},
	})
	log.Println("Listening on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
