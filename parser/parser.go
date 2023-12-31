// Package parser implements a parser for Lox source code.
package parser

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/lithammer/dedent"

	"github.com/marcuscaisey/golox/ast"
	"github.com/marcuscaisey/golox/lexer"
	"github.com/marcuscaisey/golox/token"
)

// Parse parses the source code read from r.
// If an error is returned then an incomplete AST will still be returned along with it.
func Parse(r io.Reader) (ast.Node, error) {
	l, err := lexer.New(r)
	if err != nil {
		return ast.Program{}, fmt.Errorf("constructing parser: %s", err)
	}

	p := &parser{l: l}
	errHandler := func(tok token.Token, msg string) {
		p.lastErrPos = tok.Position
		err := &syntaxError{
			tok: tok,
			msg: msg,
		}
		p.errs = append(p.errs, err)
	}
	l.SetErrorHandler(errHandler)

	return p.Parse()
}

// parser parses Lox source code into an abstract syntax tree.
type parser struct {
	l          *lexer.Lexer
	tok        token.Token // token currently being considered
	errs       []error
	lastErrPos token.Position
}

// Parse parses the source code and returns the root node of the abstract syntax tree.
// If an error is returned then an incomplete AST will still be returned along with it.
func (p *parser) Parse() (ast.Node, error) {
	p.next() // Read the first token into p.tok
	program := ast.Program{}
	for p.tok.Type != token.EOF {
		program.Stmts = append(program.Stmts, p.safelyParseStmt())
	}
	if len(p.errs) > 0 {
		return program, errors.Join(p.errs...)
	}
	return program, nil
}

func (p *parser) safelyParseStmt() (stmt ast.Stmt) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(unwind); ok {
				p.sync()
				stmt = ast.IllegalStmt{}
			} else {
				panic(r)
			}
		}
	}()
	return p.parseStmt()
}

// sync synchronises the parser with the next statement. This is used to recover from a parsing error.
func (p *parser) sync() {
	for p.tok.Type != token.EOF {
		switch p.tok.Type {
		case token.Semicolon:
			p.next()
			return
		case token.Print, token.Var, token.If, token.While, token.For, token.Function, token.Return, token.Class:
			return
		}
		p.next()
	}
}

func (p *parser) parseStmt() ast.Stmt {
	if p.tok.Type == token.Print {
		return p.parsePrintStmt()
	}
	return p.parseExprStmt()
}

func (p *parser) parsePrintStmt() ast.Stmt {
	p.expect(token.Print)
	expr := p.parseExpr()
	p.expect(token.Semicolon)
	return ast.PrintStmt{Expr: expr}
}

func (p *parser) parseExprStmt() ast.Stmt {
	expr := p.parseExpr()
	p.expect(token.Semicolon)
	return ast.ExprStmt{Expr: expr}
}

func (p *parser) parseExpr() ast.Expr {
	return p.parseCommaExpr()
}

func (p *parser) parseCommaExpr() ast.Expr {
	return p.parseBinaryExpr(p.parseTernaryExpr, token.Comma)
}

func (p *parser) parseTernaryExpr() ast.Expr {
	expr := p.parseEqualityExpr()
	if p.tok.Type == token.Question {
		p.next()
		then := p.parseExpr()
		p.expect(token.Colon)
		elseExpr := p.parseTernaryExpr()
		expr = ast.TernaryExpr{
			Condition: expr,
			Then:      then,
			Else:      elseExpr,
		}
	}
	return expr
}

func (p *parser) parseEqualityExpr() ast.Expr {
	return p.parseBinaryExpr(p.parseRelationalExpr, token.Equal, token.NotEqual)
}

func (p *parser) parseRelationalExpr() ast.Expr {
	return p.parseBinaryExpr(p.parseAdditiveExpr, token.Less, token.LessEqual, token.Greater, token.GreaterEqual)
}

func (p *parser) parseAdditiveExpr() ast.Expr {
	return p.parseBinaryExpr(p.parseMultiplicativeExpr, token.Plus, token.Minus)
}

func (p *parser) parseMultiplicativeExpr() ast.Expr {
	return p.parseBinaryExpr(p.parseUnaryExpr, token.Asterisk, token.Slash)
}

// parseBinaryExpr parses a binary expression which uses the given operators. next is a function which parses an
// expression of next highest precedence.
func (p *parser) parseBinaryExpr(next func() ast.Expr, operators ...token.Type) ast.Expr {
	expr := next()
	for slices.Contains(operators, p.tok.Type) {
		op := p.tok
		p.next()
		right := next()
		expr = ast.BinaryExpr{
			Left:  expr,
			Op:    op,
			Right: right,
		}
	}
	return expr
}

func (p *parser) parseUnaryExpr() ast.Expr {
	if p.tok.Type == token.Bang || p.tok.Type == token.Minus {
		op := p.tok
		p.next()
		right := p.parseUnaryExpr()
		return ast.UnaryExpr{
			Op:    op,
			Right: right,
		}
	}
	return p.parsePrimaryExpr()
}

func (p *parser) parsePrimaryExpr() ast.Expr {
	var expr ast.Expr
	switch tok := p.tok; tok.Type {
	case token.Number:
		value, err := strconv.ParseFloat(p.tok.Literal, 64)
		if err != nil {
			panic(fmt.Sprintf("unexpected error parsing number literal: %s", err))
		}
		expr = ast.LiteralExpr{Value: value}
	case token.String:
		value := p.tok.Literal[1 : len(p.tok.Literal)-1] // Remove surrounding quotes
		expr = ast.LiteralExpr{Value: value}
	case token.True:
		expr = ast.LiteralExpr{Value: true}
	case token.False:
		expr = ast.LiteralExpr{Value: false}
	case token.Nil:
		expr = ast.LiteralExpr{Value: nil}
	case token.LeftParen:
		p.next()
		innerExpr := p.parseExpr()
		p.expect(token.RightParen)
		expr = ast.GroupExpr{Expr: innerExpr}
		return expr
	// Error productions
	case token.Equal, token.NotEqual, token.Less, token.LessEqual, token.Greater, token.GreaterEqual, token.Asterisk, token.Slash, token.Plus:
		p.addSyntaxErrorf("binary operator %h must have left and right operands", p.tok.Type)
		p.next()
		var right ast.Expr
		switch tok.Type {
		case token.Equal, token.NotEqual:
			right = p.parseEqualityExpr()
		case token.Less, token.LessEqual, token.Greater, token.GreaterEqual:
			right = p.parseRelationalExpr()
		case token.Plus:
			right = p.parseMultiplicativeExpr()
		case token.Asterisk, token.Slash:
			right = p.parseUnaryExpr()
		}
		return ast.BinaryExpr{
			Left:  ast.IllegalExpr{},
			Op:    tok,
			Right: right,
		}
	default:
		p.expect(token.Number, token.String, token.True, token.False, token.Nil, token.LeftParen)
	}
	p.next()
	return expr
}

// expect checks that the current token has the given type and calls next if so. Otherwise, a syntax error is raised.
func (p *parser) expect(types ...token.Type) {
	for _, t := range types {
		if p.tok.Type == t {
			p.next()
			return
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "unexpected %h, expected %h", p.tok.Type, types[0])
	switch len(types) {
	case 1:
	case 2:
		fmt.Fprintf(&b, " or %h", types[1])
	default:
		for i := 1; i < len(types)-1; i++ {
			fmt.Fprintf(&b, ", %h", types[i])
		}
		fmt.Fprintf(&b, " or %h", types[len(types)-1])
	}
	p.addSyntaxErrorf(b.String())
	panic(unwind{})
}

// next reads the next token from the lexer into p.tok.
func (p *parser) next() {
	p.tok = p.l.Next()
}

func (p *parser) addSyntaxErrorf(format string, a ...any) {
	if len(p.errs) > 0 && p.tok.Position == p.lastErrPos {
		return
	}
	p.errs = append(p.errs, &syntaxError{
		tok: p.tok,
		msg: fmt.Sprintf(format, a...),
	})
}

// unwind is used as a panic value so that we can unwind the stack and recover from a parsing error without having to
// check for errors after every call to each parsing method.
type unwind struct{}

type syntaxError struct {
	tok token.Token
	msg string
}

func (e *syntaxError) Error() string {
	// If the token spans multiple lines, only show the first one. I'm not sure what the best way of highlighting and
	// pointing to a multi-line token is.
	tok, _, _ := strings.Cut(e.tok.String(), "\n")
	line := e.tok.Position.File.Line(e.tok.Position.Line)
	data := map[string]any{
		"pos":    e.tok.Position,
		"msg":    e.msg,
		"before": line[:e.tok.Position.Column-1],
		"tok":    tok,
		"after":  line[e.tok.Position.Column+len(tok)-1:],
	}
	funcs := template.FuncMap{
		"red":    color.New(color.FgRed).SprintFunc(),
		"bold":   color.New(color.Bold).SprintFunc(),
		"repeat": strings.Repeat,
	}
	text := strings.TrimSpace(dedent.Dedent(`
		{{ .pos }}: syntax error: {{ .msg }}
		{{ .before }}{{ .tok | bold | red }}{{ .after }}
		{{ repeat " " (len .before) }}{{ repeat "^" (len .tok) | red | bold }}
	`))

	tmpl := template.Must(template.New("").Funcs(funcs).Parse(text))
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}
