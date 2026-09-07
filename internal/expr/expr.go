package expr

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrSyntaxError     = errors.New("expr: syntax error")
	ErrUnexpectedToken = errors.New("expr: unexpected token")
	ErrUnsupportedType = errors.New("expr: unsupported type for operator")
	ErrDivisionByZero  = errors.New("expr: division by zero")
	ErrPathNotFound    = errors.New("expr: jsonpath key not found")
)

type TokenType int

const (
	TokEOF TokenType = iota
	TokNumber
	TokString
	TokBool
	TokNull
	TokPath
	TokIdent
	TokOp
	TokLParen
	TokRParen
	TokLBracket
	TokRBracket
	TokComma
)

type Token struct {
	Type  TokenType
	Value string
}

// Lexer converts expression string into a stream of tokens.
type Lexer struct {
	input []rune
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(input)}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) next() rune {
	ch := l.peek()
	l.pos++
	return ch
}

func (l *Lexer) NextToken() (Token, error) {
	for {
		ch := l.peek()
		if ch == 0 {
			return Token{Type: TokEOF}, nil
		}
		if unicode.IsSpace(ch) {
			l.next()
			continue
		}
		break
	}

	ch := l.peek()

	// Parentheses & Brackets
	if ch == '(' {
		l.next()
		return Token{Type: TokLParen, Value: "("}, nil
	}
	if ch == ')' {
		l.next()
		return Token{Type: TokRParen, Value: ")"}, nil
	}
	if ch == '[' {
		l.next()
		return Token{Type: TokLBracket, Value: "["}, nil
	}
	if ch == ']' {
		l.next()
		return Token{Type: TokRBracket, Value: "]"}, nil
	}
	if ch == ',' {
		l.next()
		return Token{Type: TokComma, Value: ","}, nil
	}

	// String literals (single or double quoted)
	if ch == '"' || ch == '\'' {
		quote := l.next()
		var b strings.Builder
		for {
			c := l.next()
			if c == 0 {
				return Token{}, fmt.Errorf("%w: unterminated string literal", ErrSyntaxError)
			}
			if c == quote {
				break
			}
			if c == '\\' {
				esc := l.next()
				if esc == 'n' {
					b.WriteRune('\n')
				} else if esc == 't' {
					b.WriteRune('\t')
				} else {
					b.WriteRune(esc)
				}
				continue
			}
			b.WriteRune(c)
		}
		return Token{Type: TokString, Value: b.String()}, nil
	}

	// JSONPath starting with '$'
	if ch == '$' {
		l.next()
		var b strings.Builder
		b.WriteRune('$')
		for {
			c := l.peek()
			if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '.' || c == '_' || c == '-' {
				b.WriteRune(l.next())
			} else {
				break
			}
		}
		return Token{Type: TokPath, Value: b.String()}, nil
	}

	// Numbers
	if unicode.IsDigit(ch) {
		var b strings.Builder
		hasDot := false
		for {
			c := l.peek()
			if unicode.IsDigit(c) {
				b.WriteRune(l.next())
			} else if c == '.' && !hasDot {
				hasDot = true
				b.WriteRune(l.next())
			} else {
				break
			}
		}
		return Token{Type: TokNumber, Value: b.String()}, nil
	}

	// Multi-character and single-character operators
	twoChars := ""
	if l.pos+1 < len(l.input) {
		twoChars = string(l.input[l.pos : l.pos+2])
	}
	switch twoChars {
	case "==", "!=", "<=", ">=", "&&", "||":
		l.pos += 2
		return Token{Type: TokOp, Value: twoChars}, nil
	}

	switch ch {
	case '<', '>', '+', '-', '*', '/', '!':
		l.next()
		return Token{Type: TokOp, Value: string(ch)}, nil
	}

	// Identifiers or keyword operators (in, contains, true, false, null)
	if unicode.IsLetter(ch) || ch == '_' {
		var b strings.Builder
		for {
			c := l.peek()
			if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
				b.WriteRune(l.next())
			} else {
				break
			}
		}
		word := b.String()
		lower := strings.ToLower(word)
		switch lower {
		case "true", "false":
			return Token{Type: TokBool, Value: lower}, nil
		case "null", "nil":
			return Token{Type: TokNull, Value: "null"}, nil
		case "in", "contains":
			return Token{Type: TokOp, Value: lower}, nil
		default:
			return Token{Type: TokIdent, Value: word}, nil
		}
	}

	return Token{}, fmt.Errorf("%w: unexpected character '%c'", ErrSyntaxError, ch)
}

// Node represents an AST node.
type Node interface {
	Eval(env map[string]interface{}) (interface{}, error)
}

// LiteralNode represents constants.
type LiteralNode struct {
	Value interface{}
}

func (n *LiteralNode) Eval(env map[string]interface{}) (interface{}, error) {
	return n.Value, nil
}

// ListNode represents array literals `[1, 2, 3]`.
type ListNode struct {
	Elements []Node
}

func (n *ListNode) Eval(env map[string]interface{}) (interface{}, error) {
	out := make([]interface{}, len(n.Elements))
	for i, el := range n.Elements {
		v, err := el.Eval(env)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// PathNode resolves JSONPath like `$.status` from environment map.
type PathNode struct {
	Path string
}

func (n *PathNode) Eval(env map[string]interface{}) (interface{}, error) {
	if env == nil {
		return nil, nil
	}

	p := strings.TrimPrefix(n.Path, "$.")
	p = strings.TrimPrefix(p, "$")
	segments := strings.Split(p, ".")

	var current interface{} = env
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		val, exists := m[seg]
		if !exists {
			return nil, nil
		}
		current = val
	}
	return current, nil
}

// UnaryNode represents unary operators `!`, `-`.
type UnaryNode struct {
	Op   string
	Expr Node
}

func (n *UnaryNode) Eval(env map[string]interface{}) (interface{}, error) {
	val, err := n.Expr.Eval(env)
	if err != nil {
		return nil, err
	}

	switch n.Op {
	case "!":
		return !isTruthy(val), nil
	case "-":
		num, err := toFloat(val)
		if err != nil {
			return nil, err
		}
		return -num, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrSyntaxError, n.Op)
	}
}

// BinaryNode represents binary operators `&&`, `||`, `==`, `!=`, `<`, `>`, `in`, `+`, etc.
type BinaryNode struct {
	Left  Node
	Op    string
	Right Node
}

func (n *BinaryNode) Eval(env map[string]interface{}) (interface{}, error) {
	// Short-circuiting for logical operators
	if n.Op == "||" {
		leftVal, err := n.Left.Eval(env)
		if err != nil {
			return nil, err
		}
		if isTruthy(leftVal) {
			return true, nil
		}
		rightVal, err := n.Right.Eval(env)
		if err != nil {
			return nil, err
		}
		return isTruthy(rightVal), nil
	}

	if n.Op == "&&" {
		leftVal, err := n.Left.Eval(env)
		if err != nil {
			return nil, err
		}
		if !isTruthy(leftVal) {
			return false, nil
		}
		rightVal, err := n.Right.Eval(env)
		if err != nil {
			return nil, err
		}
		return isTruthy(rightVal), nil
	}

	leftVal, err := n.Left.Eval(env)
	if err != nil {
		return nil, err
	}
	rightVal, err := n.Right.Eval(env)
	if err != nil {
		return nil, err
	}

	switch n.Op {
	case "==":
		return isEqual(leftVal, rightVal), nil
	case "!=":
		return !isEqual(leftVal, rightVal), nil
	case "<":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l < r, nil
	case "<=":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l <= r, nil
	case ">":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l > r, nil
	case ">=":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l >= r, nil
	case "+":
		if ls, ok := leftVal.(string); ok {
			return ls + fmt.Sprintf("%v", rightVal), nil
		}
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l + r, nil
	case "-":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l - r, nil
	case "*":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		return l * r, nil
	case "/":
		l, r, err := toFloatPair(leftVal, rightVal)
		if err != nil {
			return nil, err
		}
		if r == 0 {
			return nil, ErrDivisionByZero
		}
		return l / r, nil
	case "in":
		return isIn(leftVal, rightVal), nil
	case "contains":
		return isContains(leftVal, rightVal), nil
	default:
		return nil, fmt.Errorf("%w: unknown operator %s", ErrSyntaxError, n.Op)
	}
}

func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int, int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	default:
		return true
	}
}

func toFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: cannot convert string %q to number", ErrUnsupportedType, val)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("%w: value %v cannot be converted to number", ErrUnsupportedType, v)
	}
}

func toFloatPair(left, right interface{}) (float64, float64, error) {
	l, err := toFloat(left)
	if err != nil {
		return 0, 0, err
	}
	r, err := toFloat(right)
	if err != nil {
		return 0, 0, err
	}
	return l, r, nil
}

func isEqual(left, right interface{}) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	// Try float comparison
	lf, errL := toFloat(left)
	rf, errR := toFloat(right)
	if errL == nil && errR == nil {
		return lf == rf
	}
	return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
}

func isIn(item, collection interface{}) bool {
	if collection == nil {
		return false
	}
	switch coll := collection.(type) {
	case []interface{}:
		for _, el := range coll {
			if isEqual(item, el) {
				return true
			}
		}
	case []string:
		strItem := fmt.Sprintf("%v", item)
		for _, el := range coll {
			if strItem == el {
				return true
			}
		}
	case string:
		return strings.Contains(coll, fmt.Sprintf("%v", item))
	}
	return false
}

func isContains(collection, item interface{}) bool {
	return isIn(item, collection)
}

// Parser builds AST from tokens.
type Parser struct {
	tokens []Token
	pos    int
}

func Parse(input string) (Node, error) {
	lexer := NewLexer(input)
	var tokens []Token
	for {
		t, err := lexer.NextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
		if t.Type == TokEOF {
			break
		}
	}
	p := &Parser{tokens: tokens}
	node, err := p.parseLogicalOr()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokEOF {
		return nil, fmt.Errorf("%w: unexpected trailing token %s", ErrSyntaxError, p.peek().Value)
	}
	return node, nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) next() Token {
	t := p.peek()
	p.pos++
	return t
}

func (p *Parser) parseLogicalOr() (Node, error) {
	left, err := p.parseLogicalAnd()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TokOp && p.peek().Value == "||" {
		op := p.next().Value
		right, err := p.parseLogicalAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryNode{Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *Parser) parseLogicalAnd() (Node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TokOp && p.peek().Value == "&&" {
		op := p.next().Value
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &BinaryNode{Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *Parser) parseComparison() (Node, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TokOp {
		op := p.peek().Value
		if op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" || op == "in" || op == "contains" {
			p.next()
			right, err := p.parseAddSub()
			if err != nil {
				return nil, err
			}
			left = &BinaryNode{Left: left, Op: op, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *Parser) parseAddSub() (Node, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TokOp {
		op := p.peek().Value
		if op == "+" || op == "-" {
			p.next()
			right, err := p.parseMulDiv()
			if err != nil {
				return nil, err
			}
			left = &BinaryNode{Left: left, Op: op, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *Parser) parseMulDiv() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for p.peek().Type == TokOp {
		op := p.peek().Value
		if op == "*" || op == "/" {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &BinaryNode{Left: left, Op: op, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Node, error) {
	if p.peek().Type == TokOp && (p.peek().Value == "!" || p.peek().Value == "-") {
		op := p.next().Value
		sub, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &UnaryNode{Op: op, Expr: sub}, nil
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (Node, error) {
	t := p.peek()

	switch t.Type {
	case TokNumber:
		p.next()
		f, _ := strconv.ParseFloat(t.Value, 64)
		return &LiteralNode{Value: f}, nil
	case TokString:
		p.next()
		return &LiteralNode{Value: t.Value}, nil
	case TokBool:
		p.next()
		return &LiteralNode{Value: t.Value == "true"}, nil
	case TokNull:
		p.next()
		return &LiteralNode{Value: nil}, nil
	case TokPath:
		p.next()
		return &PathNode{Path: t.Value}, nil
	case TokLParen:
		p.next()
		sub, err := p.parseLogicalOr()
		if err != nil {
			return nil, err
		}
		if p.next().Type != TokRParen {
			return nil, fmt.Errorf("%w: missing closing parenthesis", ErrSyntaxError)
		}
		return sub, nil
	case TokLBracket:
		p.next()
		var elements []Node
		for p.peek().Type != TokRBracket {
			el, err := p.parseLogicalOr()
			if err != nil {
				return nil, err
			}
			elements = append(elements, el)
			if p.peek().Type == TokComma {
				p.next()
			} else if p.peek().Type != TokRBracket {
				return nil, fmt.Errorf("%w: expected comma or closing bracket", ErrSyntaxError)
			}
		}
		p.next() // consume ']'
		return &ListNode{Elements: elements}, nil
	default:
		return nil, fmt.Errorf("%w: unexpected token %s", ErrUnexpectedToken, t.Value)
	}
}

// Evaluate evaluates expression string against the payload environment.
func Evaluate(exprStr string, env map[string]interface{}) (interface{}, error) {
	ast, err := Parse(exprStr)
	if err != nil {
		return nil, err
	}
	return ast.Eval(env)
}

// EvaluateBool evaluates expression string and asserts a boolean result.
func EvaluateBool(exprStr string, env map[string]interface{}) (bool, error) {
	res, err := Evaluate(exprStr, env)
	if err != nil {
		return false, err
	}
	return isTruthy(res), nil
}
