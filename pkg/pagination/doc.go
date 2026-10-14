// Package pagination 提供框架无关的分页参数定义。
//
// 本包是 PageRequest 的唯一定义位置，middleware 和 data/gormx 均依赖本包，
// 避免 middleware 层反向依赖 data 层。
package pagination
