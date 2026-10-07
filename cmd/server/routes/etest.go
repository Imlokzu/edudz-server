package routes

import (
	"io"
	"net/http"

	"github.com/DislikesSchool/EduPage2-server/edupage"
	"github.com/gin-gonic/gin"
)

// ETestHandler returns the raw e-test / homework card data (text, widgets, files).
// @Router /api/etest [get]
func ETestHandler(c *gin.Context) {
	client := c.MustGet("client").(*edupage.EdupageClient)

	data, err := client.FetchETestData(c.Query("testid"), c.Query("superid"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

// FileHandler proxies an EduPage attachment through the user's session.
// @Router /api/file [get]
func FileHandler(c *gin.Context) {
	client := c.MustGet("client").(*edupage.EdupageClient)

	resp, err := client.FetchFileContext(c.Request.Context(), c.Query("src"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Disposition"} {
		if v := resp.Header.Get(h); v != "" {
			c.Header(h, v)
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}
