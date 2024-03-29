// Package parser implements a parser for Lox source code.
package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"

	"github.com/marcuscaisey/golox/ast"
	"github.com/marcuscaisey/golox/lexer"
	"github.com/marcuscaisey/golox/token"
)

// Parse parses the source code read from r.
// If an error is returned then an incomplete AST will still be returned along with it.
func Parse(r io.Reader) (ast.Program, error) {
	l, err := lexer.New(r)
	if err != nil {
		return ast.Program{}, fmt.Errorf("constructing parser: %s", err)
	}

	p := &parser{l: l}
	errHandler := func(tok token.Token, msg string) {
		p.lastErrPos = tok.Start
		err := &syntaxError{
			start: tok.Start,
			end:   tok.End,
			msg:   msg,
		}
		p.errs = append(p.errs, err)
	}
	l.SetErrorHandler(errHandler)

	return p.Parse()
}

// parser parses Lox source code into an abstract syntax tree.
type parser struct {
	l         *lexer.Lexer
	tok       token.Token // token currently being considered
	loopDepth int

	errs       []error
	lastErrPos token.Position
}

// Parse parses the source code and returns the root node of the abstract syntax tree.
// If an error is returned then an incomplete AST will still be returned along with it.
func (p *parser) Parse() (ast.Program, error) {
	p.next() // Advance to the first token
	program := ast.Program{}
	for p.tok.Type != token.EOF {
		program.Stmts = append(program.Stmts, p.safelyParseDecl())
	}
	if len(p.errs) > 0 {
		return program, errors.Join(p.errs...)
	}
	return program, nil
}

func (p *parser) safelyParseDecl() (stmt ast.Stmt) {
	from := p.tok
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(unwind); ok {
				to := p.sync()
				stmt = ast.IllegalStmt{From: from, To: to}
			} else {
				panic(r)
			}
		}
	}()
	return p.parseDecl()
}

// sync synchronises the parser with the next statement. This is used to recover from a parsing error.
// The final token before the next statement is returned.
func (p *parser) sync() token.Token {
	finalTok := p.tok
	for {
		switch p.tok.Type {
		case token.Semicolon:
			finalTok := p.tok
			p.next()
			return finalTok
		case token.Print, token.Var, token.If, token.LeftBrace, token.While, token.For, token.Break, token.Continue, token.EOF:
			return finalTok
		}
		finalTok = p.tok
		p.next()
	}
}

func (p *parser) parseDecl() ast.Stmt {
	switch tok := p.tok; {
	case p.match(token.Var):
		return p.parseVarDecl(tok)
	default:
		return p.parseStmt()
	}
}

func (p *parser) parseVarDecl(varTok token.Token) ast.Stmt {
	name := p.expect(token.Ident, "%h must be followed by a variable name", token.Var)
	var value ast.Expr
	if p.match(token.Equal) {
		value = p.parseExpr("on right-hand side of variable declaration")
	}
	semicolon := p.expectSemicolon("after variable declaration")
	return ast.VarDecl{Var: varTok, Name: name, Initialiser: value, Semicolon: semicolon}
}

func (p *parser) parseStmt() ast.Stmt {
	switch tok := p.tok; {
	case p.match(token.Print):
		return p.parsePrintStmt(tok)
	case p.match(token.LeftBrace):
		return p.parseBlock(tok)
	case p.match(token.If):
		return p.parseIfStmt(tok)
	case p.match(token.While):
		return p.parseWhileStmt(tok)
	case p.match(token.For):
		return p.parseForStmt(tok)
	case p.match(token.Break):
		return p.parseBreakStmt(tok)
	case p.match(token.Continue):
		return p.parseContinueStmt(tok)
	default:
		return p.parseExprStmt()
	}
}

func (p *parser) parseExprStmt() ast.Stmt {
	expr := p.parseExpr("in expression statement")
	semicolon := p.expectSemicolon("after expression statement")
	return ast.ExprStmt{Expr: expr, Semicolon: semicolon}
}

func (p *parser) parsePrintStmt(printTok token.Token) ast.Stmt {
	expr := p.parseExpr("in print statement")
	semicolon := p.expectSemicolon("after print statement")
	return ast.PrintStmt{Print: printTok, Expr: expr, Semicolon: semicolon}
}

func (p *parser) parseBlock(leftBrace token.Token) ast.Stmt {
	var stmts []ast.Stmt
	for p.tok.Type != token.RightBrace && p.tok.Type != token.EOF {
		stmts = append(stmts, p.safelyParseDecl())
	}
	rightBrace := p.expect(token.RightBrace, "expected closing %h after block", token.RightBrace)
	return ast.BlockStmt{LeftBrace: leftBrace, Stmts: stmts, RightBrace: rightBrace}
}

func (p *parser) parseIfStmt(ifTok token.Token) ast.Stmt {
	p.expect(token.LeftParen, "%h should be followed by condition inside %h%h", token.If, token.LeftParen, token.RightParen)
	condition := p.parseExpr("as condition in if statement")
	p.expect(token.RightParen, "%h should be followed by condition inside %h%h", token.If, token.LeftParen, token.RightParen)
	thenBranch := p.parseStmt()
	var elseBranch ast.Stmt
	if p.match(token.Else) {
		elseBranch = p.parseStmt()
	}
	return ast.IfStmt{If: ifTok, Condition: condition, Then: thenBranch, Else: elseBranch}
}

func (p *parser) parseWhileStmt(whileTok token.Token) ast.Stmt {
	p.loopDepth++
	defer func() { p.loopDepth-- }()
	p.expect(token.LeftParen, "%h should be followed by condition inside %h%h", token.While, token.LeftParen, token.RightParen)
	condition := p.parseExpr("as condition in while statement")
	p.expect(token.RightParen, "%h should be followed by condition inside %h%h", token.While, token.LeftParen, token.RightParen)
	body := p.parseStmt()
	return ast.WhileStmt{While: whileTok, Condition: condition, Body: body}
}

func (p *parser) parseForStmt(forTok token.Token) ast.Stmt {
	p.loopDepth++
	defer func() { p.loopDepth-- }()
	p.expect(token.LeftParen, "%h should be followed by initialise statement, condition expression, and update expression inside %h%h", token.For, token.LeftParen, token.RightParen)
	var initialise ast.Stmt
	switch tok := p.tok; {
	case p.match(token.Var):
		initialise = p.parseVarDecl(tok)
	case p.match(token.Semicolon):
	default:
		initialise = p.parseExprStmt()
	}
	var condition ast.Expr
	if !p.match(token.Semicolon) {
		condition = p.parseExpr("as condition in for loop")
		p.expectSemicolon("after for loop condition")
	}
	var update ast.Expr
	if !p.match(token.RightParen) {
		update = p.parseExpr("as update expression in for loop")
		p.expect(token.RightParen, "%h should be followed by initialise statement, condition expression, and update expression inside %h%h", token.For, token.LeftParen, token.RightParen)
	}
	body := p.parseStmt()
	return ast.ForStmt{For: forTok, Initialise: initialise, Condition: condition, Update: update, Body: body}
}

func (p *parser) parseBreakStmt(breakTok token.Token) ast.Stmt {
	semicolon := p.expectSemicolon("after break statement")
	stmt := ast.BreakStmt{Break: breakTok, Semicolon: semicolon}
	if p.loopDepth == 0 {
		p.addNodeErrorf(stmt, "break statement must be inside a loop")
	}
	return stmt
}

func (p *parser) parseContinueStmt(continueTok token.Token) ast.Stmt {
	semicolon := p.expectSemicolon("after continue statement")
	stmt := ast.ContinueStmt{Continue: continueTok, Semicolon: semicolon}
	if p.loopDepth == 0 {
		p.addNodeErrorf(stmt, "continue statement must be inside a loop")
	}
	return stmt
}

func (p *parser) parseExpr(context string) ast.Expr {
	return p.parseCommaExpr(context)
}

func (p *parser) parseCommaExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseAssignmentExpr, token.Comma)
}

func (p *parser) parseAssignmentExpr(context string) ast.Expr {
	expr := p.parseTernaryExpr(context)
	if p.match(token.Equal) {
		left, ok := expr.(ast.VariableExpr)
		if !ok {
			p.addNodeErrorf(expr, "left-hand side of assignment must be a variable")
		}
		right := p.parseAssignmentExpr("on right-hand side of assignment")
		expr = ast.AssignmentExpr{
			Left:  left.Name,
			Right: right,
		}
	}
	return expr
}

func (p *parser) parseTernaryExpr(context string) ast.Expr {
	expr := p.parseLogicalOrExpr(context)
	if p.match(token.Question) {
		then := p.parseExpr("for then part of ternary expression")
		p.expect(token.Colon, "next part of ternary expression should be %h", token.Colon)
		elseExpr := p.parseTernaryExpr("for else part of ternary expression")
		expr = ast.TernaryExpr{
			Condition: expr,
			Then:      then,
			Else:      elseExpr,
		}
	}
	return expr
}

func (p *parser) parseLogicalOrExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseLogicalAndExpr, token.Or)
}

func (p *parser) parseLogicalAndExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseEqualityExpr, token.And)
}

func (p *parser) parseEqualityExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseRelationalExpr, token.EqualEqual, token.BangEqual)
}

func (p *parser) parseRelationalExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseAdditiveExpr, token.Less, token.LessEqual, token.Greater, token.GreaterEqual)
}

func (p *parser) parseAdditiveExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseMultiplicativeExpr, token.Plus, token.Minus)
}

func (p *parser) parseMultiplicativeExpr(context string) ast.Expr {
	return p.parseBinaryExpr(context, p.parseUnaryExpr, token.Asterisk, token.Slash, token.Percent)
}

// parseBinaryExpr parses a binary expression which uses the given operators. next is a function which parses an
// expression of next highest precedence.
func (p *parser) parseBinaryExpr(context string, next func(context string) ast.Expr, operators ...token.Type) ast.Expr {
	expr := next(context)
	for {
		op, ok := p.match2(operators...)
		if !ok {
			break
		}
		right := next("on right-hand side of binary expression")
		expr = ast.BinaryExpr{
			Left:  expr,
			Op:    op,
			Right: right,
		}
	}
	return expr
}

func (p *parser) parseUnaryExpr(context string) ast.Expr {
	if op, ok := p.match2(token.Bang, token.Minus); ok {
		right := p.parseUnaryExpr("after unary operator")
		return ast.UnaryExpr{
			Op:    op,
			Right: right,
		}
	}
	return p.parsePrimaryExpr(context)
}

func (p *parser) parsePrimaryExpr(context string) ast.Expr {
	var expr ast.Expr
	switch tok := p.tok; tok.Type {
	case token.Number, token.String, token.True, token.False, token.Nil:
		expr = ast.LiteralExpr{Value: tok}
	case token.LeftParen:
		leftParen := tok
		p.next()
		innerExpr := p.parseExpr("inside parentheses")
		rightParen := p.expect(token.RightParen, "expected closing %h after opening %h at %s", token.RightParen, token.LeftParen, leftParen.Start)
		return ast.GroupExpr{LeftParen: leftParen, Expr: innerExpr, RightParen: rightParen}
	case token.Ident:
		expr = ast.VariableExpr{Name: tok}
	// Error productions
	case token.EqualEqual, token.BangEqual, token.Less, token.LessEqual, token.Greater, token.GreaterEqual, token.Asterisk, token.Slash, token.Plus:
		p.addTokenErrorf(tok, "binary operator %h must have left and right operands", tok.Type)
		p.next()
		var right ast.Expr
		switch tok.Type {
		case token.EqualEqual, token.BangEqual:
			right = p.parseEqualityExpr("on right-hand side of binary expression")
		case token.Less, token.LessEqual, token.Greater, token.GreaterEqual:
			right = p.parseRelationalExpr("on right-hand side of binary expression")
		case token.Plus:
			right = p.parseMultiplicativeExpr("on right-hand side of binary expression")
		case token.Asterisk, token.Slash:
			right = p.parseUnaryExpr("on right-hand side of binary expression")
		}
		return ast.BinaryExpr{
			Op:    tok,
			Right: right,
		}
	default:
		p.addTokenErrorf(tok, "expected expression "+context)
		panic(unwind{})
	}
	p.next()
	return expr
}

// match returns whether the current token is one of the given types and advances the parser if so.
func (p *parser) match(types ...token.Type) bool {
	for _, t := range types {
		if p.tok.Type == t {
			p.next()
			return true
		}
	}
	return false
}

// match2 is like match but also returns the matched token.
func (p *parser) match2(types ...token.Type) (token.Token, bool) {
	tok := p.tok
	return tok, p.match(types...)
}

// expect returns the current token and advances the parser if it has the given type. Otherwise, a syntax error is
// raised with the given format and arguments.
func (p *parser) expect(t token.Type, format string, a ...any) token.Token {
	if p.tok.Type == t {
		tok := p.tok
		p.next()
		return tok
	}
	if p.tok.Type == token.EOF {
		format += " but end of file was reached"
	} else {
		format += ", found %h"
		a = append(a, p.tok.Type)
	}
	p.addTokenErrorf(p.tok, format, a...)
	panic(unwind{})
}

func (p *parser) expectSemicolon(context string) token.Token {
	return p.expect(token.Semicolon, "expected %h %s", token.Semicolon, context)
}

// next advances the parser to the next token.
func (p *parser) next() {
	p.tok = p.l.Next()
}

func (p *parser) addSyntaxErrorf(start, end token.Position, format string, a ...any) {
	if len(p.errs) > 0 && start == p.lastErrPos {
		return
	}
	p.errs = append(p.errs, &syntaxError{
		start: start,
		end:   end,
		msg:   fmt.Sprintf(format, a...),
	})
}

func (p *parser) addTokenErrorf(tok token.Token, format string, a ...any) {
	p.addSyntaxErrorf(tok.Start, tok.End, format, a...)
}

func (p *parser) addNodeErrorf(node ast.Node, format string, a ...any) {
	p.addSyntaxErrorf(node.Start(), node.End(), format, a...)
}

// unwind is used as a panic value so that we can unwind the stack and recover from a parsing error without having to
// check for errors after every call to each parsing method.
type unwind struct{}

type syntaxError struct {
	start token.Position
	end   token.Position
	msg   string
}

func (e *syntaxError) Error() string {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed)

	var b strings.Builder
	buildString := func() string {
		return strings.TrimSuffix(b.String(), "\n")
	}

	bold.Fprintln(&b, e.start, ": ", red.Sprint("syntax error: "), e.msg)

	lines := make([]string, e.end.Line-e.start.Line+1)
	for i := e.start.Line; i <= e.end.Line; i++ {
		line := e.start.File.Line(i)
		if !utf8.Valid(line) {
			// If any of the lines are not valid UTF-8 then we can't display the source code, so just return the error
			// message on its own. This is a very rare case and it's not worth the effort to handle it any better.
			return buildString()
		}
		lines[i-e.start.Line] = string(line)
	}
	fmt.Fprintln(&b, string(lines[0]))
	if e.start == e.end {
		// There's nothing to highlight
		return buildString()
	}

	if len(lines) == 1 {
		fmt.Fprint(&b, strings.Repeat(" ", runewidth.StringWidth(string(lines[0][:e.start.Column]))))
		red.Fprintln(&b, strings.Repeat("~", runewidth.StringWidth(string(lines[0][e.start.Column:e.end.Column]))))
	} else {
		fmt.Fprint(&b, strings.Repeat(" ", runewidth.StringWidth(string(lines[0][:e.start.Column]))))
		red.Fprintln(&b, strings.Repeat("~", runewidth.StringWidth(string(lines[0][e.start.Column:]))))
		for _, line := range lines[1 : len(lines)-1] {
			fmt.Fprintln(&b, string(line))
			red.Fprintln(&b, strings.Repeat("~", runewidth.StringWidth(string(line))))
		}
		if lastLine := lines[len(lines)-1]; len(lastLine) > 0 {
			fmt.Fprintln(&b, string(lastLine))
			red.Fprintln(&b, strings.Repeat("~", runewidth.StringWidth(string(lastLine[:e.end.Column]))))
		}
	}

	return buildString()
}
