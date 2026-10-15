// Package network 提供网络相关的轻量工具函数。
//
// 本包只包含 IP 地址解析与获取（IsPublicIP/GetPodIP 等）这类零依赖的
// 基础工具，供 registry、nns 等模块在解析注册地址时复用；HTTP 客户端
// （httpx）、WebSocket（wsutil）、消息队列（nats/mq）等网络客户端见
// github.com/oekit/network。
package network
