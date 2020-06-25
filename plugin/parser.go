package plugin

import (
	"strconv"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// AccountNumber is a special struct that acts as a conditional string/int type
type AccountNumber string

// UnmarshalYAML handles the unmarshalling of the AccountNumber type
func (n *AccountNumber) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		anum := AccountNumber(str)
		*n = anum
		return nil
	}
	var num int
	if err := unmarshal(&num); err != nil {
		logrus.Errorf("There's an error: %s", err)
		return err
	}
	anum := AccountNumber(strconv.Itoa(num))
	*n = anum
	return nil
}

// bellyjay1005 is the struct for .strithon.yml files
type bellyjay1005 struct {
	Kind     string
	Metadata struct {
		Service struct {
			ID     string   `yaml:"id"`
			Name   string   `yaml:"name"`
			Team   string   `yaml:"team"`
			Unit   string   `yaml:"unit"`
			Owners []string `yaml:"owners"`
			MSTeam struct {
				Name    string `yaml:"name"`
				Channel string `yaml:"channel"`
			} `yaml:"ms_team"`
			Description string `yaml:"description"`
		} `yaml:"service"`
		Environments []struct {
			Name    string        `yaml:"name"`
			Cloud   string        `yaml:"cloud"`
			Account AccountNumber `yaml:"account"`
			Region  string        `yaml:"region"`
		} `yaml:"environments",omitempty`
	} `yaml:"metadata"`
}

// ParsestrithonYml loads a .strithon.yml into the bellyjay1005 struct
func ParsestrithonYml(s string) (*bellyjay1005, error) {
	contents := []byte(s)
	var bellyjay1005Config bellyjay1005
	err := yaml.Unmarshal(contents, &bellyjay1005Config)
	if err != nil {
		return nil, err
	}

	return &bellyjay1005Config, nil
}
