package onebotfilter

import (
	"log"

	regexp "github.com/dlclark/regexp2"
)

type Filter struct {
	Name    string
	filters []*regexp.Regexp
}

var (
	allFilters []*Filter
)

func AddFilter(filter *Filter) {
	for _, f := range allFilters {
		if f.Name == filter.Name {
			return
		}
	}
	allFilters = append(allFilters, filter)
}
func RemoveFilter(name string) {
	for i, f := range allFilters {
		if f.Name == name {
			allFilters = append(allFilters[:i], allFilters[i+1:]...)
			return
		}
	}
}
func ReLoadFilters() {
	for _, whitelistClient := range CONFIG.Whitelist {
		for _, filter := range allFilters {
			if whitelistClient.Name == filter.Name {
				filter.Load(whitelistClient.Filters)
				log.Printf("%s已重新加载白名单：%s\n", filter.Name, filter.String())
				break
			}
		}
	}
	for _, blacklistClient := range CONFIG.Blacklist {
		for _, filter := range allFilters {
			if blacklistClient.Name == filter.Name {
				filter.Load(blacklistClient.Filters)
				log.Printf("%s已重新加载黑名单：%s\n", filter.Name, filter.String())
				break
			}
		}
	}
}

func (f *Filter) Load(filters []string) *Filter {
	f.filters = []*regexp.Regexp{}
	for _, filter := range filters {
		pattern, err := regexp.Compile(filter, regexp.None)
		if err != nil {
			log.Printf("编译正则表达式：%s，出错：%v\n", filter, err)
			continue
		}
		f.filters = append(f.filters, pattern)
	}
	return f
}
func (f *Filter) Filter(str string) bool {
	for _, pattern := range f.filters {
		if ok, err := pattern.MatchString(str); ok {
			return true
		} else if err != nil {
			log.Printf("%s的过滤器%s正则匹配出错的消息：%s\n", f.Name, pattern.String(), str)
		}
	}
	return false
}
func (f *Filter) String() string {
	str := "[ "
	for _, pattern := range f.filters {
		str += pattern.String() + ", "
	}
	if str != "[ " {
		str = str[:len(str)-2]
	}
	str += " ]"
	return str
}
