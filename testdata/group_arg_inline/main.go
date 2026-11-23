package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/group_arg_inline/mod"
	"github.com/gin-gonic/gin"
)

// Reply is the response body every handler in this fixture returns.
type Reply struct {
	Name string `json:"name"`
}

func main() {
	r := gin.Default()
	v1 := r.Group("/v1")

	// Shape 1: group created and used at the same site.
	v1.Group("/inline").GET("/a", handleA)

	// Shape 2: group assigned to a variable, then passed to a registrar.
	g := v1.Group("/viavar")
	registerViaVar(g)

	// Shape 3: the parent group is passed, the callee creates the subgroup.
	registerNested(v1)

	// Shape 4: the group is created inline in the argument list, same package.
	registerSamePkg(v1.Group("/samepkg"))

	// Shape 5: the same, with the registrar in another package — the module
	// wiring style this fixture exists for.
	mod.RegisterRouter(v1.Group("/mod"))

	// Shape 6: the group is created inline on the ENGINE, not on a group.
	registerEng(r.Group("/eng"))

	// Shape 7: engine group via a variable (control).
	eg := r.Group("/engvar")
	registerEngVar(eg)

	// Shape 8: a user type embedding the engine; Group is promoted twice.
	app := &App{Engine: r}
	registerApp(app.Group("/app"))
	app.Group("/appdirect").GET("/z", handleG)

	_ = r.Run(":8080")
}

// App embeds the engine so Group reaches it through a promoted method.
type App struct{ *gin.Engine }

func registerEng(g *gin.RouterGroup)    { g.GET("/e", handleE) }
func registerEngVar(g *gin.RouterGroup) { g.GET("/f", handleF) }
func registerApp(g *gin.RouterGroup)    { g.GET("/g", handleG) }

func registerViaVar(r *gin.RouterGroup) {
	r.GET("/d", handleD)
}

func registerNested(r *gin.RouterGroup) {
	r.Group("/nested").GET("/c", handleC)
}

func registerSamePkg(r *gin.RouterGroup) {
	r.GET("/b", handleB)
}

func handleA(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "a"}) }
func handleB(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "b"}) }
func handleC(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "c"}) }
func handleD(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "d"}) }
func handleE(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "e"}) }
func handleF(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "f"}) }
func handleG(c *gin.Context) { c.JSON(http.StatusOK, Reply{Name: "g"}) }
