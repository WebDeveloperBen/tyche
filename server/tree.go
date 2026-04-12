package server

import (
	"fmt"
	"strings"
	"sync"
)

var paramValuePool = sync.Pool{
	New: func() any {
		values := make([]string, 0, 4)
		return &values
	},
}

func getParamValues() []string {
	return (*paramValuePool.Get().(*[]string))[:0]
}

func putParamValues(values []string) {
	values = values[:0]
	paramValuePool.Put(&values)
}

type node struct {
	part    string
	isParam bool
	isWC    bool
	isRoot  bool
	wcName  string

	parent *node
	colon  *node
	// Indexed children for O(1) lookup
	children    []*node
	childrenIdx map[string]*node
	handlers    [9]*routeHandler
	route       string
}

type routeHandler struct {
	fn        HandlerFunc
	params    []string
	wildcard  string
	group     *Group
	wrappedFn HandlerFunc
}

const (
	methodGet = iota
	methodHead
	methodPost
	methodPut
	methodPatch
	methodDelete
	methodConnect
	methodOptions
	methodTrace
)

func methodIndex(method string) int {
	switch len(method) {
	case 3:
		if method[0] == 'G' && method[1] == 'E' && method[2] == 'T' {
			return methodGet
		}
		if method[0] == 'P' && method[1] == 'U' && method[2] == 'T' {
			return methodPut
		}
	case 4:
		switch method[0] {
		case 'H':
			if method[1] == 'E' && method[2] == 'A' && method[3] == 'D' {
				return methodHead
			}
		case 'P':
			if method[1] == 'O' && method[2] == 'S' && method[3] == 'T' {
				return methodPost
			}
		}
	case 5:
		switch method[0] {
		case 'P':
			if method[1] == 'A' && method[2] == 'T' && method[3] == 'C' && method[4] == 'H' {
				return methodPatch
			}
		case 'T':
			if method[1] == 'R' && method[2] == 'A' && method[3] == 'C' && method[4] == 'E' {
				return methodTrace
			}
		}
	case 6:
		if method[0] == 'D' && method[1] == 'E' && method[2] == 'L' && method[3] == 'E' && method[4] == 'T' && method[5] == 'E' {
			return methodDelete
		}
	case 7:
		switch method[0] {
		case 'C':
			if method[1] == 'O' && method[2] == 'N' && method[3] == 'N' && method[4] == 'E' && method[5] == 'C' && method[6] == 'T' {
				return methodConnect
			}
		case 'O':
			if method[1] == 'P' && method[2] == 'T' && method[3] == 'I' && method[4] == 'O' && method[5] == 'N' && method[6] == 'S' {
				return methodOptions
			}
		}
	}
	return -1
}

func (n *node) addRoute(route string) *node {
	if route == "/" {
		n.isRoot = true
		return n
	}

	parts := splitRouteFast(route)
	curr := n

	for _, part := range parts {
		if len(part) > 0 && part[0] == ':' {
			if curr.colon == nil {
				curr.colon = &node{part: part, isParam: true, parent: curr}
			} else {
				curr.colon.part = part
			}
			curr = curr.colon
		} else if len(part) > 0 && part[0] == '*' {
			curr.isWC = true
			curr.wcName = part[1:]
		} else {
			child := curr.childrenIdx[part]
			if child == nil {
				child = &node{part: part, parent: curr}
				curr.children = append(curr.children, child)
				if curr.childrenIdx == nil {
					curr.childrenIdx = make(map[string]*node, 4)
				}
				curr.childrenIdx[part] = child
			}
			curr = child
		}
	}

	curr.route = route
	curr.isRoot = true
	return curr
}

func splitRouteFast(route string) []string {
	if route == "/" {
		return nil
	}
	n := len(route)
	if n < 2 || route[0] != '/' {
		panic("invalid route: " + route)
	}

	segCount := 1
	for i := 1; i < n; i++ {
		if route[i] == '/' {
			segCount++
		}
	}

	parts := make([]string, 0, segCount)
	start := 1
	for i := 1; i < n; i++ {
		if route[i] == '/' {
			parts = append(parts, route[start:i])
			start = i + 1
		}
	}
	parts = append(parts, route[start:])
	return parts
}

type findResult struct {
	handler       *routeHandler
	paramValues   []string
	wildcard      string
	hasWildcard   bool
	routeExists   bool
	methodAllowed bool
}

func (n *node) find(method string, path string) findResult {
	if len(path) == 0 || path[0] != '/' {
		return findResult{}
	}

	idx := methodIndex(method)
	if idx < 0 {
		return findResult{routeExists: false}
	}

	if path == "/" {
		if h := n.handlers[idx]; h != nil {
			return findResult{handler: h, routeExists: true, methodAllowed: true}
		}
		if n.hasAnyHandler() {
			return findResult{routeExists: true, methodAllowed: false}
		}
		return findResult{routeExists: false}
	}

	return n._find(idx, path[1:], 0)
}

func (n *node) _find(idx int, path string, depth int) findResult {
	if path == "" {
		if h := n.handlers[idx]; h != nil {
			return findResult{handler: h, routeExists: true, methodAllowed: true}
		}
		if n.handlers[idx] == nil && n.hasAnyHandler() {
			return findResult{routeExists: true, methodAllowed: false}
		}
		if n.isRoot && n.childrenIdx == nil {
			return findResult{routeExists: n.hasAnyHandler()}
		}
		return findResult{routeExists: false}
	}

	before, after, ok := strings.Cut(path, "/")
	var part, rest string
	if ok {
		part = before
		rest = after
	} else {
		part = path
		rest = ""
	}

	foundChild := false

	if n.childrenIdx != nil {
		if child := n.childrenIdx[part]; child != nil {
			foundChild = true
			result := child._find(idx, rest, depth)
			if result.handler != nil {
				return result
			}
			if result.routeExists {
				return result
			}
		}
	}

	if n.colon != nil {
		result := n.colon._find(idx, rest, depth+1)
		if result.handler != nil {
			if result.paramValues == nil {
				result.paramValues = getParamValues()
			}
			result.paramValues = append(result.paramValues, part)
			return result
		}
		if result.routeExists {
			return result
		}
	}

	if n.isWC {
		if h := n.handlers[idx]; h != nil {
			return findResult{handler: h, wildcard: path, hasWildcard: true, routeExists: true, methodAllowed: true}
		}
		if n.parent != nil && n.parent.handlers[idx] != nil {
			return findResult{handler: n.parent.handlers[idx], wildcard: path, hasWildcard: true, routeExists: true, methodAllowed: true}
		}
		if n.hasAnyHandler() || (n.parent != nil && n.parent.hasAnyHandler()) {
			return findResult{routeExists: true, methodAllowed: n.handlers[idx] != nil}
		}
	}

	if rest == "" && foundChild {
		isRoot := n.parent == nil
		if (isRoot && n.childrenIdx == nil) || (!isRoot && n.childrenIdx != nil) {
			if h := n.handlers[idx]; h != nil {
				return findResult{handler: h, routeExists: true, methodAllowed: true}
			}
			if n.hasAnyHandler() {
				return findResult{routeExists: true, methodAllowed: false}
			}
		}
	}

	if n.parent == nil && n.childrenIdx == nil {
		return findResult{routeExists: n.hasAnyHandler()}
	}
	return findResult{routeExists: false}
}

func (n *node) hasAnyHandler() bool {
	for _, h := range n.handlers {
		if h != nil {
			return true
		}
	}
	return false
}

func (n *node) setHandler(method, route string, fn HandlerFunc, params []string, wildcard string, group *Group, wrappedFn HandlerFunc) error {
	idx := methodIndex(method)
	if idx >= 0 {
		if n.handlers[idx] != nil {
			return fmt.Errorf("duplicate route handler registered for %s %s", method, route)
		}
		n.handlers[idx] = &routeHandler{fn: fn, params: params, wildcard: wildcard, group: group, wrappedFn: wrappedFn}
	}
	return nil
}

func (n *node) rebuildHandlers(wrap func(*routeHandler) HandlerFunc) {
	for _, handler := range n.handlers {
		if handler == nil || handler.fn == nil || handler.group == nil {
			continue
		}
		handler.wrappedFn = wrap(handler)
	}
	if n.colon != nil {
		n.colon.rebuildHandlers(wrap)
	}
	for _, child := range n.children {
		child.rebuildHandlers(wrap)
	}
}
