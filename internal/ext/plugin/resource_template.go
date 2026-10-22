package plugin

import (
	"context"
	"regexp"
	"strings"
)

// A resource template is an RFC 6570 URI template a server declares for
// addresses it builds on demand. Only literal text, simple expansion ({name})
// and reserved expansion ({+name}) are understood: any other operator makes the
// template match nothing, so the host never admits an address by guessing what
// an operator it does not read would have produced.
const (
	templateSimpleValue   = `[A-Za-z0-9._~%-]+`
	templateReservedValue = `[A-Za-z0-9._~%:/?#\[\]@!$&'()*+,;=-]+`
)

func (c *Client) listResourceTemplates(ctx context.Context) ([]string, error) {
	listed, err := listAllPages[struct {
		URITemplate string `json:"uriTemplate"`
	}](ctx, c, "resources/templates/list", "resourceTemplates")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(listed))
	for _, t := range listed {
		out = append(out, t.URITemplate)
	}
	return out, nil
}

// uriMatchesTemplate reports whether uri is something the template can expand
// to. A uri with whitespace or control characters never matches.
func uriMatchesTemplate(template, uri string) bool {
	if uri == "" || strings.IndexFunc(uri, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
		return false
	}
	var pattern strings.Builder
	pattern.WriteString("^")
	rest := template
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			pattern.WriteString(regexp.QuoteMeta(rest))
			break
		}
		pattern.WriteString(regexp.QuoteMeta(rest[:open]))
		closeAt := strings.IndexByte(rest[open:], '}')
		if closeAt < 0 {
			return false
		}
		expr := rest[open+1 : open+closeAt]
		rest = rest[open+closeAt+1:]
		value := templateSimpleValue
		if after, ok := strings.CutPrefix(expr, "+"); ok {
			expr, value = after, templateReservedValue
		}
		if expr == "" || strings.ContainsAny(expr[:1], "#./;?&=,!@|") || strings.ContainsAny(expr, "{}") {
			return false
		}
		pattern.WriteString(value)
	}
	pattern.WriteString("$")
	re, err := regexp.Compile(pattern.String())
	return err == nil && re.MatchString(uri)
}
