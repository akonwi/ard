package checker

import (
	"fmt"

	"github.com/akonwi/ard/ast"
)

func (c *checker) validateFiberFunction(fnNode ast.Expression) {
	switch node := fnNode.(type) {
	case *ast.AnonymousFunction:
		c.checkBlock(node.Body, func() {
			// do not inherit parent scope
			c.scope.isolate()
		})
	case *ast.FunctionDeclaration:
		{
			c.checkBlock(node.Body, func() {
				// do not inherit parent scope
				c.scope.isolate()
			})
		}
	default:
		// probably need to handle when the function is a variable reference
		panic(fmt.Sprintf("Unhandled fiber function node: %T", node))
	}

}
