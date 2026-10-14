package pagination

// PageResponse 是全库统一的分页响应信封。
//
// 此前 data/gormx（JSON "list"）与 data/crud（JSON "items"）各有一个
// 字段名/JSON 契约不同的泛型分页结构，导致 API 契约分叉；两者已统一
// 为本类型的类型别名，新代码直接使用 pagination.PageResponse。
//
// 字段取舍说明：数据字段名采用 items（与 handler/crudh 的 REST 契约
// 一致），pages 保留 gormx 语义（总页数，向上取整）。
type PageResponse[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}
