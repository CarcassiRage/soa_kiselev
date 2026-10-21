package handler

import (
	"errors"
	"hw2/db_handler"
	"hw2/internal/api"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	ProductsDB *db_handler.ProductsDB
}

func NewHandler(productsDB db_handler.ProductsDB) *Handler {
	return &Handler{
		ProductsDB: &productsDB,
	}
}

func (h *Handler) CreateProduct(c *gin.Context) {
	var input api.ProductCreate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error_code": "VALIDATION_ERROR",
			"message":    err.Error(),
		})
		return
	}
	product, err := h.ProductsDB.Create(
		c.Request.Context(),
		input,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to create product",
		})
		return
	}
	c.JSON(http.StatusCreated, product)
}

func (h *Handler) GetProduct(c *gin.Context, id int64) {
	product, err := h.ProductsDB.Get(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error_code": "PRODUCT_NOT_FOUND",
			"message":    "Product not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to get product",
		})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (h *Handler) ListProducts(c *gin.Context, params api.ListProductsParams) {
	page := 0
	size := 20
	if params.Page != nil {
		page = *params.Page
	}
	if params.Size != nil {
		size = *params.Size
	}

	result, err := h.ProductsDB.List(c.Request.Context(), page, size, params.Status, params.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to list products",
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) UpdateProduct(c *gin.Context, id int64) {
	var input api.ProductUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error_code": "VALIDATION_ERROR",
			"message":    err.Error(),
		})
		return
	}
	product, err := h.ProductsDB.Update(
		c.Request.Context(), id, input,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error_code": "PRODUCT_NOT_FOUND",
			"message":    "Product not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to update product",
		})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (h *Handler) DeleteProduct(c *gin.Context, id int64) {
	err := h.ProductsDB.Delete(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error_code": "PRODUCT_NOT_FOUND",
			"message":    "Product not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error_code": "INTERNAL_ERROR",
			"message":    "Failed to archive product",
		})
		return
	}
	c.Status(http.StatusNoContent)
}
