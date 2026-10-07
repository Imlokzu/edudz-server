package routes

import (
	"context"
	"fmt"
	"github.com/DislikesSchool/EduPage2-server/edupage"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func LessonPlanHandler(c *gin.Context) {
	date, err := time.Parse("2006-01-02", c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_date"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	client := c.MustGet("client").(*edupage.EdupageClient)
	plan, err := client.LessonPlan(ctx, date)
	if err != nil {
		fmt.Printf("edudz lesson plan unavailable: %v\n", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "lesson_plan_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plan": plan})
}
