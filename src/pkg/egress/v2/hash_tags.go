package v2

import (
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
)

func HashTags(tags map[string]string) string {
	hash := ""
	elements := []mapElement{}
	for k, v := range tags {
		elements = append(elements, mapElement{k, v})
	}
	sort.Sort(byKey(elements))
	for _, element := range elements {
		kHash, vHash := sha256.New(), sha256.New()
		io.WriteString(kHash, element.k)
		io.WriteString(vHash, element.v)
		hash += fmt.Sprintf("%x%x", kHash.Sum(nil), vHash.Sum(nil))
	}
	return hash
}

type byKey []mapElement

func (a byKey) Len() int           { return len(a) }
func (a byKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byKey) Less(i, j int) bool { return a[i].k < a[j].k }

type mapElement struct {
	k, v string
}
