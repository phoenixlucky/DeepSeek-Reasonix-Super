package surfacespec

import "encoding/json"

func (s *Spec) Redact(redact func(string) string) {
	s.Title = redact(s.Title)
	var walk func([]Node)
	walk = func(nodes []Node) {
		for i := range nodes {
			n := &nodes[i]
			n.Value = redact(n.Value)
			n.Label = redact(n.Label)
			if n.Kind == "kv" {
				n.Key = redact(n.Key)
			}
			if n.Field != nil {
				for j := range n.Field.Options {
					o := &n.Field.Options[j]
					o.Label = redact(o.Label)
					o.Description = redact(o.Description)
					o.Preview = redact(o.Preview)
				}
				if n.Field.Type == "string" && len(n.Field.Default) != 0 {
					var value string
					if json.Unmarshal(n.Field.Default, &value) == nil {
						n.Field.Default, _ = json.Marshal(redact(value))
					}
				}
			}
			walk(n.Children)
		}
	}
	walk(s.Body)
}
