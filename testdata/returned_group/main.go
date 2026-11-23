// Fixture: a gin group created inside a helper and returned to the caller,
// which registers on it (issue #633). The group's prefix comes from the
// Group() call inside the helper, so the caller's routes must be documented
// under it:
//
//	/ident/x          g := r.Group("/ident"); return g
//	/ident/again      the same helper called again: one group, both routes
//	/direct/x         return r.Group("/direct")
//	/hop/x            Outer returns Inner(r), which returns r.Group("/hop")
//	/api/nested/x     api := r.Group("/api"); return api.Group("/nested")
//	/public/v/a       Version(pub) — one helper, two callers with different
//	/admin/v/b        Version(adm)   parents; neither route leaks into the other
//	/passed/reg       the returned group passed on to a registering function
//	/witherr/x        a (group, error) helper
//	/wpub/w1          Wrap(parent) returns Identity(parent): the parameter is
//	/wadm/w2            forwarded, and each call resolves to its own argument
//	/wtwo/w3          Wrap2 forwards through Wrap: two hops of forwarding
//	/vx/via/one       Via groups on a local holding its forwarded parameter
//	/lpub/lv/l1       LocalWrap returns Identity(g) for its own local group g:
//	/ladm/lv/l2         followed in each invocation, not shared by both
//	/lpub/lw/v/m1     LocalVersion returns Version(g) for its own local group:
//	/ladm/lw/v/m2       the same, one helper further
//	/fpub/lf/v/f1     LocalFwd returns Fwd(g), which returns Version(p), for its
//	/fadm/lf/v/f2       own local group g: two helpers further
//	/opub/lf/v/o1     OuterP forwards to LocalFwd: the first caller's group
//
// Negative cases:
//
//	/fresh            New() returns a fresh engine that nothing mounts
//	/kept/same        Same(g) returns its parameter: the caller's prefix only
//	/pick             Pick returns one of two groups: no single prefix to choose
//	/reassigned       Reassigned returns a local it reassigns on one path
//	/rec              Rec returns its own call: no group to follow, and the
//	                  route must not be bound beneath a recursive copy of it
//	/sa, /sb          Suffixed(vp, "/a") and Suffixed(vp, "/b") return
//	                  parent.Group(suffix) under the same parent: the moved
//	                  Group is one node for both calls, so neither suffix can
//	                  be told apart; no prefix rather than a merged one
//	/o, /p            the same shared Group, reached first through a group
//	                  passed on to RegO and only then through a second
//	                  Suffixed call: the answer must not depend on which
//	                  statement comes first
//	/swap, /swapret   Swap and SwapRet reassign the parameter they return on
//	                  one path: not the caller's argument on every path
//	/t1, /t2          ViaTwice is Via called from two parents: the local's
//	                  producer depends on the invocation, and the claim on it
//	                  cannot say which, so neither parent is applied
//	/swapgroup        SwapGroup groups on a parameter it reassigns on one path
//	/ba, /bb          Branch(bp, "/a") and Branch(Identity(bp), "/b"): two
//	                  keys for the one Group node moved under /bp
//	/aa, /ab          Area(ap, "/a") and Area(aq, "/b"): distinct parents, but
//	                  the moved Group reads its suffix from neither call
//	/o2               OuterP's second caller: the Group LocalFwd makes inside
//	                  OuterP is bound under the first caller's group only
//	/o3               OuterP of an inline group (not followed yet)
//	/lc1, /lc2        LocalCallArg passes Fwd an inline Group of its own: one
//	                  node per invocation, which nothing places for either
//	/gs, /gsret, /gsv GroupSwap, GroupSwapRet and GroupSwapV assign their
//	                  parameter a group of another one on one path: called
//	                  with swap false, the value is never that group
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func h(c *gin.Context) { c.Status(http.StatusOK) }

func Ident(r *gin.Engine) *gin.RouterGroup { g := r.Group("/ident"); return g }

func Direct(r *gin.Engine) *gin.RouterGroup { return r.Group("/direct") }

func Inner(r *gin.Engine) *gin.RouterGroup { return r.Group("/hop") }

func Outer(r *gin.Engine) *gin.RouterGroup { return Inner(r) }

func Nested(r *gin.Engine) *gin.RouterGroup {
	api := r.Group("/api")
	return api.Group("/nested")
}

func Version(parent *gin.RouterGroup) *gin.RouterGroup { return parent.Group("/v") }

func New() *gin.Engine { return gin.New() }

func Same(g *gin.RouterGroup) *gin.RouterGroup { return g }

func Passed(r *gin.Engine) *gin.RouterGroup { return r.Group("/passed") }

func register(g *gin.RouterGroup) { g.GET("/reg", h) }

func WithErr(r *gin.Engine) (*gin.RouterGroup, error) {
	g := r.Group("/witherr")
	return g, nil
}

func Pick(r *gin.Engine, alt bool) *gin.RouterGroup {
	if alt {
		return r.Group("/alt")
	}
	return r.Group("/main")
}

func Reassigned(r *gin.Engine, alt bool) *gin.RouterGroup {
	g := r.Group("/first")
	if alt {
		g = r.Group("/second")
	}
	return g
}

func Rec(r *gin.Engine) *gin.RouterGroup { return Rec(r) }

func Identity(g *gin.RouterGroup) *gin.RouterGroup { return g }

func Wrap(parent *gin.RouterGroup) *gin.RouterGroup { return Identity(parent) }

func Wrap2(p *gin.RouterGroup) *gin.RouterGroup { return Wrap(p) }

func Via(parent *gin.RouterGroup) *gin.RouterGroup {
	g := Identity(parent)
	return g.Group("/via")
}

func ViaTwice(parent *gin.RouterGroup) *gin.RouterGroup {
	g := Identity(parent)
	return g.Group("/via")
}

func Suffixed(parent *gin.RouterGroup, suffix string) *gin.RouterGroup {
	return parent.Group(suffix)
}

func Mk(r *gin.Engine) *gin.RouterGroup { return r.Group("/mk") }

func RegO(g *gin.RouterGroup) { g.GET("/o", h) }

func Swap(parent, other *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		parent = other
	}
	return Identity(parent)
}

func SwapRet(parent, other *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		parent = other
	}
	return parent
}

func LocalWrap(parent *gin.RouterGroup) *gin.RouterGroup {
	g := parent.Group("/lv")
	return Identity(g)
}

func LocalVersion(parent *gin.RouterGroup) *gin.RouterGroup {
	g := parent.Group("/lw")
	return Version(g)
}

func SwapGroup(parent, other *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		parent = other
	}
	return parent.Group("/leaf")
}

func GroupSwap(p, q *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		p = q.Group("/other")
	}
	return Identity(p)
}

func GroupSwapRet(p, q *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		p = q.Group("/other")
	}
	return p
}

func GroupSwapV(p, q *gin.RouterGroup, swap bool) *gin.RouterGroup {
	if swap {
		p = q.Group("/other")
	}
	return Version(p)
}

func Branch(parent *gin.RouterGroup, suffix string) *gin.RouterGroup {
	return parent.Group(suffix)
}

func Area(parent *gin.RouterGroup, suffix string) *gin.RouterGroup {
	return parent.Group(suffix)
}

func Fwd(p *gin.RouterGroup) *gin.RouterGroup { return Version(p) }

func LocalFwd(parent *gin.RouterGroup) *gin.RouterGroup {
	g := parent.Group("/lf")
	return Fwd(g)
}

func OuterP(parent *gin.RouterGroup) *gin.RouterGroup { return LocalFwd(parent) }

func LocalCallArg(parent *gin.RouterGroup) *gin.RouterGroup { return Fwd(parent.Group("/ic")) }

func main() {
	r := gin.New()

	ident := Ident(r)
	ident.GET("/x", h)
	again := Ident(r)
	again.GET("/again", h)

	direct := Direct(r)
	direct.GET("/x", h)

	hop := Outer(r)
	hop.GET("/x", h)

	nested := Nested(r)
	nested.GET("/x", h)

	pub := r.Group("/public")
	adm := r.Group("/admin")
	a := Version(pub)
	a.GET("/a", h)
	b := Version(adm)
	b.GET("/b", h)

	fresh := New()
	fresh.GET("/fresh", h)

	kept := r.Group("/kept")
	same := Same(kept)
	same.GET("/same", h)

	passed := Passed(r)
	register(passed)

	we, err := WithErr(r)
	if err != nil {
		return
	}
	we.GET("/x", h)

	pick := Pick(r, true)
	pick.GET("/pick", h)

	re := Reassigned(r, true)
	re.GET("/reassigned", h)

	rec := Rec(r)
	rec.GET("/rec", h)

	wpub := r.Group("/wpub")
	wadm := r.Group("/wadm")
	w1 := Wrap(wpub)
	w1.GET("/w1", h)
	w2 := Wrap(wadm)
	w2.GET("/w2", h)
	wtwo := r.Group("/wtwo")
	w3 := Wrap2(wtwo)
	w3.GET("/w3", h)

	vx := r.Group("/vx")
	via := Via(vx)
	via.GET("/one", h)

	ty := r.Group("/ty")
	tz := r.Group("/tz")
	t1 := ViaTwice(ty)
	t1.GET("/t1", h)
	t2 := ViaTwice(tz)
	t2.GET("/t2", h)

	vp := r.Group("/vp")
	sa := Suffixed(vp, "/a")
	sa.GET("/sa", h)
	sb := Suffixed(vp, "/b")
	sb.GET("/sb", h)

	mk := Mk(r)
	oa := Suffixed(mk, "/oa")
	RegO(oa)
	ob := Suffixed(mk, "/ob")
	ob.GET("/p", h)

	sp := r.Group("/sp")
	so := r.Group("/so")
	sw := Swap(sp, so, true)
	sw.GET("/swap", h)
	sr := SwapRet(sp, so, true)
	sr.GET("/swapret", h)
	sg := SwapGroup(sp, so, true)
	sg.GET("/swapgroup", h)

	lpub := r.Group("/lpub")
	ladm := r.Group("/ladm")
	l1 := LocalWrap(lpub)
	l1.GET("/l1", h)
	l2 := LocalWrap(ladm)
	l2.GET("/l2", h)
	m1 := LocalVersion(lpub)
	m1.GET("/m1", h)
	m2 := LocalVersion(ladm)
	m2.GET("/m2", h)

	bp := r.Group("/bp")
	ba := Branch(bp, "/a")
	ba.GET("/ba", h)
	bb := Branch(Identity(bp), "/b")
	bb.GET("/bb", h)

	ap := r.Group("/ap")
	aq := r.Group("/aq")
	aa := Area(ap, "/a")
	aa.GET("/aa", h)
	ab := Area(aq, "/b")
	ab.GET("/ab", h)

	fpub := r.Group("/fpub")
	fadm := r.Group("/fadm")
	f1 := LocalFwd(fpub)
	f1.GET("/f1", h)
	f2 := LocalFwd(fadm)
	f2.GET("/f2", h)

	opg := r.Group("/opub")
	oag := r.Group("/oadm")
	o1 := OuterP(opg)
	o1.GET("/o1", h)
	o2 := OuterP(oag)
	o2.GET("/o2", h)
	o3 := OuterP(r.Group("/oin"))
	o3.GET("/o3", h)

	lc1 := LocalCallArg(r.Group("/lca"))
	lc1.GET("/lc1", h)
	lc2 := LocalCallArg(r.Group("/lcb"))
	lc2.GET("/lc2", h)

	gp := r.Group("/gp")
	gq := r.Group("/gq")
	gs := GroupSwap(gp, gq, false)
	gs.GET("/gs", h)
	gsr := GroupSwapRet(gp, gq, false)
	gsr.GET("/gsret", h)
	gsv := GroupSwapV(gp, gq, false)
	gsv.GET("/gsv", h)

	_ = r.Run(":8080")
}
