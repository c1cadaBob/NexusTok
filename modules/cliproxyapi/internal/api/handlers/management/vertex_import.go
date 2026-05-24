// management - vertex_import.go
// Vertex AI 服务账号凭据导入端点。
// 该模块处理 Vertex AI 服务账号 JSON 文件的上传和导入，
// 将其转换为认证记录并保存到认证目录中。
package management

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/vertex"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// ImportVertexCredential 处理 Vertex AI 服务账号 JSON 文件的上传。
// 处理流程：
//  1. 从 multipart 表单中读取上传的文件
//  2. 解析并验证 JSON 格式
//  3. 标准化服务账号字段
//  4. 提取 project_id、client_email 和 location
//  5. 创建认证记录并保存到认证目录
//
// 支持通过表单字段或查询参数指定 location，默认为 "us-central1"。
func (h *Handler) ImportVertexCredential(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config unavailable"})
		return
	}
	if h.cfg.AuthDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth directory not configured"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to read file: %v", err)})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to read file: %v", err)})
		return
	}

	var serviceAccount map[string]any
	if err := json.Unmarshal(data, &serviceAccount); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json", "message": err.Error()})
		return
	}

	normalizedSA, err := vertex.NormalizeServiceAccountMap(serviceAccount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service account", "message": err.Error()})
		return
	}
	serviceAccount = normalizedSA

	projectID := strings.TrimSpace(valueAsString(serviceAccount["project_id"]))
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id missing"})
		return
	}
	email := strings.TrimSpace(valueAsString(serviceAccount["client_email"]))

	location := strings.TrimSpace(c.PostForm("location"))
	if location == "" {
		location = strings.TrimSpace(c.Query("location"))
	}
	if location == "" {
		location = "us-central1"
	}

	fileName := fmt.Sprintf("vertex-%s.json", sanitizeVertexFilePart(projectID))
	label := labelForVertex(projectID, email)
	storage := &vertex.VertexCredentialStorage{
		ServiceAccount: serviceAccount,
		ProjectID:      projectID,
		Email:          email,
		Location:       location,
		Type:           "vertex",
	}
	metadata := map[string]any{
		"service_account": serviceAccount,
		"project_id":      projectID,
		"email":           email,
		"location":        location,
		"type":            "vertex",
		"label":           label,
	}
	record := &coreauth.Auth{
		ID:       fileName,
		Provider: "vertex",
		FileName: fileName,
		Storage:  storage,
		Label:    label,
		Metadata: metadata,
	}

	ctx := context.Background()
	if reqCtx := c.Request.Context(); reqCtx != nil {
		ctx = reqCtx
	}
	savedPath, err := h.saveTokenRecord(ctx, record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save_failed", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"auth-file":  savedPath,
		"project_id": projectID,
		"email":      email,
		"location":   location,
	})
}

// valueAsString 将任意值转换为字符串。
// nil 返回空字符串，字符串类型直接返回，其他类型使用 fmt.Sprint 转换。
func valueAsString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// sanitizeVertexFilePart 清理用于文件名的字符串。
// 替换路径分隔符和特殊字符为安全的字符（下划线或连字符）。
// 空字符串返回默认值 "vertex"。
func sanitizeVertexFilePart(s string) string {
	out := strings.TrimSpace(s)
	replacers := []string{"/", "_", "\\", "_", ":", "_", " ", "-"}
	for i := 0; i < len(replacers); i += 2 {
		out = strings.ReplaceAll(out, replacers[i], replacers[i+1])
	}
	if out == "" {
		return "vertex"
	}
	return out
}

// labelForVertex 生成 Vertex 凭据的显示标签。
// 格式为 "project_id (email)"，仅有部分信息时相应简化。
func labelForVertex(projectID, email string) string {
	p := strings.TrimSpace(projectID)
	e := strings.TrimSpace(email)
	if p != "" && e != "" {
		return fmt.Sprintf("%s (%s)", p, e)
	}
	if p != "" {
		return p
	}
	if e != "" {
		return e
	}
	return "vertex"
}
