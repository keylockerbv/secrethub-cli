package tpl

import (
	"bytes"
	"errors"
	"io"
	"unicode"
)

// Errors
var (
	ErrTemplateVarNotFound      = tplError.Code("template_var_not_found").ErrorPref("no value was supplied for template variable '%s'")
	ErrUnexpectedDollar         = tplError.Code("unexpected_character").ErrorPref("unexpected '$' at line %d column %d. Use '\\$' if you want to output a dollar sign.")
	ErrIllegalVariableCharacter = tplError.Code("illegal_variable_character").ErrorPref("Illegal character '%s' at line %d column %d. Variable names can only contain letters, digits and underscores.")
	ErrIllegalSecretCharacter   = tplError.Code("illegal_secret_character").ErrorPref("Illegel character '%s' at line %d column %d. Secret paths can only contain letters, digits, underscores, hypens, dots, slashes and a colon.")
	ErrSecretTagNotClosed       = tplError.Code("secret_tag_not_closed").ErrorPref("Expected the closing of a secret tag `}}` at line %d column %d, but reached the end of the template.")
	ErrVariableTagNotClosed     = tplError.Code("variable_tag_not_closed").ErrorPref("Expected the closing of a variable tag `}` at line %d column %d, but reached the end of the template.")

	specialChars = []rune{'$', '{', '}', '\\'}
)

// NewV2Parser returns a parser for the v2 template syntax.
//
// V2 templates can contain secret paths between brackets:
// {{ path/to/secret }}
//
// Within secret paths, variables can be used. Variables are
// given between `${` and `}`.
// For example:
// {{ ${app}/db/secret }}
// Variables cannot be used outside of secret paths.
//
// Spaces directly after opening delimiters (`{{` and `${`) and directly
// before closing delimiters (`}}`, `}`) are ignored. They are not
// included in the secret pahts and variable names.
func NewV2Parser() Parser {
	return parserV2{}
}

type context struct {
	vars         map[string]string
	secretReader SecretReader
}

func (ctx context) secret(path string) (string, error) {
	return ctx.secretReader.ReadSecret(path)
}

type node interface {
	evaluate(ctx context) (string, error)
}

type secret struct {
	path []node
}

func (s secret) evaluate(ctx context) (string, error) {
	var buffer bytes.Buffer
	for _, p := range s.path {
		eval, err := p.evaluate(ctx)
		if err != nil {
			return "", err
		}
		buffer.WriteString(eval)
	}
	return ctx.secret(buffer.String())
}

type variable struct {
	key string
}

func (v variable) evaluate(ctx context) (string, error) {
	res, ok := ctx.vars[v.key]
	if !ok {
		return "", ErrTemplateVarNotFound(v.key)
	}
	return res, nil
}

type character rune

func (c character) evaluate(ctx context) (string, error) {
	return string(c), nil
}

type templateV2 struct {
	nodes []node
}

type parserV2 struct{}

// Parse parses a secret template from a raw string.
//
// A secret template can contain references to secrets in secret tags.
// A secret tag is enclosed in double brackets: `{{ <path> }}`.
//
// A secret template can contain references to variables in variable tags.
// A variable tag is enclosed in `${ <variable key> }`.
//
// Secret tags can contain variable tags:
// `{{ path/with/${var}/to/secret }}`
//
// Extra spaces can be added just after the opening delimiter and just before the closing delimiter of a tag:
// {{ path/to/secret }} has the same output as {{path/to/secret}} has.
//
// Variable tags cannot contain secret tags.
// Secret tags cannot contain secret tags (they cannot be nested).
// Variable tags cannot contain variable tags (they cannot be nested).
func (p parserV2) Parse(raw string, line, column int) (Template, error) {
	parser := newV2Parser(bytes.NewBufferString(raw), line, column)
	nodes, err := parser.parse()
	if err != nil {
		return nil, err
	}
	return templateV2{
		nodes: nodes,
	}, nil
}

func newV2Parser(buf *bytes.Buffer, line, column int) v2Parser {
	return v2Parser{
		buf:    buf,
		lineNo: line,
		// The column number indicates the index (starting at 1) of the current rune.
		// We subtract 2 of the given value. One bacause we have not read the current rune yet and
		// one more because we are reading the next rune in advance (which we don't want to count).
		columnNo: column - 2,
	}
}

type v2Parser struct {
	buf      *bytes.Buffer
	lineNo   int
	columnNo int

	current rune
	next    rune
}

// readRune reads the next rune from the raw template.
func (p *v2Parser) readRune() error {
	p.current = p.next
	if p.current == '\n' {
		p.lineNo++
		p.columnNo = 0
	} else {
		p.columnNo++
	}

	var err error
	p.next, _, err = p.buf.ReadRune()
	return err
}

func (p *v2Parser) parse() ([]node, error) {
	res := []node{}
	err := p.readRune()
	if err == io.EOF {
		return res, nil
	}
	if err != nil {
		return nil, err
	}

	for {
		err := p.readRune()
		if err == io.EOF {
			return append(res, character(p.current)), nil
		}
		if err != nil {
			return nil, err
		}

		switch p.current {
		case '$':
			switch p.next {
			case '{':
				err = p.readRune()
				if err == io.EOF {
					return res, ErrVariableTagNotClosed(p.lineNo, p.columnNo+1)
				}
				if err != nil {
					return nil, err
				}

				variable, err := p.parseVar()
				if err != nil {
					return nil, err
				}
				res = append(res, variable)

				err = p.readRune()
				if err == io.EOF {
					return res, nil
				}
				if err != nil {
					return nil, err
				}

				continue
			default:
				// We don't allow dollars before letters and underscores now,
				// as we might want to use these for $var support (without brackets) later.
				if unicode.IsLetter(p.next) || p.next == '_' {
					return nil, ErrUnexpectedDollar(p.lineNo, p.columnNo)
				}
				res = append(res, character(p.current))
				continue
			}
		case '{':
			switch p.next {
			case '{':
				secret, err := p.parseSecret()
				if err != nil {
					return nil, err
				}
				res = append(res, secret)

				err = p.readRune()
				if err == io.EOF {
					return res, nil
				}
				if err != nil {
					return nil, err
				}
				continue
			default:
				res = append(res, character(p.current))
				continue
			}
		case '\\':
			isSpecialChar := false
			for _, specialChar := range specialChars {
				if p.next == specialChar {
					isSpecialChar = true
					break
				}
			}
			if isSpecialChar {
				res = append(res, character(p.next))
				err = p.readRune()
				if err == io.EOF {
					return res, nil
				}
				if err != nil {
					return nil, err
				}
			} else {
				res = append(res, character(p.current))
			}
			continue
		default:
			res = append(res, character(p.current))
			continue
		}
	}
}

// parseVar parses the contents of a template variable up to the closing delimiter.
// parseVar should be called after the opening delimiter has been read. The next
// character from the buffer should be the first character of the contents.
//
// when parseVar returns, the next character in the buffer is the first character
// after the closing delimiter of the template variable.
func (p *v2Parser) parseVar() (node, error) {
	var buffer bytes.Buffer

	for p.next == ' ' {
		err := p.readRune()
		if err == io.EOF {
			return nil, ErrVariableTagNotClosed(p.lineNo, p.columnNo+1)
		}
		if err != nil {
			return nil, err
		}
	}

	for {
		switch p.next {
		case '}':
			return variable{
				key: buffer.String(),
			}, nil
		case ' ':
			errIllegalVariableSpace := ErrIllegalVariableCharacter(p.next, p.lineNo, p.columnNo+1)
			err := p.forwardToClosing([]rune("}"))
			if err == io.EOF {
				return nil, ErrVariableTagNotClosed(p.lineNo, p.columnNo+1)
			}
			if err != nil {
				return nil, errIllegalVariableSpace
			}
			return variable{
				key: buffer.String(),
			}, nil
		default:
			if unicode.IsLetter(p.next) || unicode.IsDigit(p.next) || p.current == '_' {
				buffer.WriteRune(p.next)

				err := p.readRune()
				if err == io.EOF {
					return nil, ErrVariableTagNotClosed(p.lineNo, p.columnNo+1)
				}
				if err != nil {
					return nil, err
				}
				continue
			}
			return nil, ErrIllegalVariableCharacter(p.next, p.lineNo, p.columnNo+1)
		}
	}
}

// parseSecret parses the contents of a secret tag up to the closing delimiter.
// parseSecret should be called after the opening delimiter has been read. The next
// character from the buffer should be the first character of the contents.
//
// when parseSecret returns, the next character in the buffer is the first character
// after the closing delimiter of the secret tag.
func (p *v2Parser) parseSecret() (node, error) {
	path := []node{}
	err := p.readRune()
	if err == io.EOF {
		return nil, ErrSecretTagNotClosed(p.lineNo, p.columnNo+1)
	}
	if err != nil {
		return nil, err
	}
	for p.next == ' ' {
		err = p.readRune()
		if err == io.EOF {
			return nil, ErrSecretTagNotClosed(p.lineNo, p.columnNo+1)
		}
		if err != nil {
			return nil, err
		}
	}

	for {
		err = p.readRune()
		if err == io.EOF {
			return nil, ErrSecretTagNotClosed(p.lineNo, p.columnNo+1)
		}
		if err != nil {
			return nil, err
		}

		switch p.current {
		case '$':
			switch p.next {
			case '{':
				err = p.readRune()
				if err == io.EOF {
					return nil, ErrVariableTagNotClosed(p.lineNo, p.columnNo+1)
				}
				if err != nil {
					return nil, err
				}
				variable, err := p.parseVar()
				if err != nil {
					return nil, err
				}
				path = append(path, variable)

				err = p.readRune()
				if err == io.EOF {
					return nil, ErrSecretTagNotClosed(p.lineNo, p.columnNo+1)
				}
				if err != nil {
					return nil, err
				}
			default:
				return nil, ErrIllegalSecretCharacter(p.current, p.lineNo, p.columnNo)
			}
		case ' ':
			err := p.forwardToClosing([]rune("}}"))
			if err != nil {
				return nil, ErrIllegalSecretCharacter(p.current, p.lineNo, p.columnNo)
			}
			return secret{
				path: path,
			}, nil
		case '}':
			switch p.next {
			case '}':
				return secret{
					path: path,
				}, nil
			default:
				return nil, ErrIllegalSecretCharacter(p.current, p.lineNo, p.columnNo)
			}
		default:
			if unicode.IsLetter(p.current) || unicode.IsDigit(p.current) || p.current == '_' || p.current == '-' || p.current == '.' || p.current == '/' || p.current == ':' {
				path = append(path, character(p.current))
				continue
			}
			return nil, ErrIllegalSecretCharacter(p.current, p.lineNo, p.columnNo)
		}
	}
}

// forwardToClosing skips all spaces up to the closing delimiter.
// It returns an error when characters other than spaces occur before the complete
// closing delimiter occurs.
func (p *v2Parser) forwardToClosing(delim []rune) error {
	if len(delim) == 0 {
		return errors.New("delim should be at least one character long")
	}
	for p.next == ' ' {
		err := p.readRune()
		if err != nil {
			return err
		}
	}
	i := 0
	for {
		if p.next != delim[i] {
			return errors.New("expected end delimiter")
		}
		i++
		if i < len(delim) {
			err := p.readRune()
			if err != nil {
				return err
			}
		} else {
			return nil
		}
	}
}

// SecretReader fetches a secret by its path.
type SecretReader interface {
	ReadSecret(path string) (string, error)
}

// Evaluate renders a template. It replaces all variable- and secret tags in the template.
func (t templateV2) Evaluate(vars map[string]string, sr SecretReader) (string, error) {
	ctx := context{
		vars:         vars,
		secretReader: sr,
	}

	var buffer bytes.Buffer
	for _, n := range t.nodes {
		eval, err := n.evaluate(ctx)
		if err != nil {
			return "", err
		}
		buffer.WriteString(eval)
	}
	return buffer.String(), nil
}
