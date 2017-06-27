package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/cloudfoundry-incubator/spiff/compare"
	"github.com/cloudfoundry-incubator/spiff/yaml"
	vaultapi "github.com/hashicorp/vault/api"
	flags "github.com/jessevdk/go-flags"
)

var interpolationRegex = regexp.MustCompile(`\(\((!?[-\.\w\pL]+)\)\)`)

type Command struct {
	Before string `long:"before" description:"Pipeline config before."`
	After  string `long:"after" description:"Pipeline config after."`
	Params string `long:"params" description:"File containing original params."`

	PathPrefix   string `long:"path-prefix" default:"/concourse" description:"Path under which credentials will be namespaced."`
	TeamName     string `long:"team-name" required:"true" description:"Team under which to set credentials."`
	PipelineName string `long:"pipeline-name" required:"true" description:"Pipeline under which to set credentials."`

	TeamParams []string `long:"team-param" description:"Chuck credential under team scope, rather than pipeline. Can be specified multiple times."`

	DryRun bool `long:"dry-run" short:"n" description:"Don't actually write to vault."`
}

func (cmd *Command) Execute(args []string) error {
	beforeBytes, err := ioutil.ReadFile(cmd.Before)
	if err != nil {
		return err
	}

	afterBytes, err := ioutil.ReadFile(cmd.After)
	if err != nil {
		return err
	}

	paramsBytes, err := ioutil.ReadFile(cmd.Params)
	if err != nil {
		return err
	}

	before, err := yaml.Parse(cmd.Before, beforeBytes)
	if err != nil {
		return err
	}

	paramsNode, err := yaml.Parse(cmd.Params, paramsBytes)
	if err != nil {
		return err
	}

	params, ok := paramsNode.Value().(map[string]yaml.Node)
	if !ok {
		log.Fatalf("params must be map[string]interface{}; got %T\n", paramsNode.Value())
	}

	after, err := yaml.Parse(cmd.After, afterBytes)
	if err != nil {
		return err
	}

	vaultClient, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		return err
	}

	vaultValues := map[string]map[string]interface{}{}

	diffs := compare.Compare(before, after)
	for _, diff := range diffs {
		path := strings.Join(diff.Path, ".")

		if diff.A == nil || diff.B == nil {
			checkOrphaned(path, diff.A, diff.B)
			checkOrphaned(path, diff.B, diff.A)

			log.Println("skipping (unrelated key change):", path)
			continue
		}

		strA, ok := diff.A.Value().(string)
		if !ok {
			continue
		}

		strB, ok := diff.B.Value().(string)
		if !ok {
			continue
		}

		aParam := interpolationRegex.FindStringSubmatch(strA)
		if aParam == nil {
			log.Println("skipping (not a parameter in A):", path)
			continue
		}

		bParam := interpolationRegex.FindStringSubmatch(strB)
		if bParam == nil {
			log.Println("skipping (not a parameter in B):", path)
			continue
		}

		value, ok := params[aParam[1]]
		if !ok {
			log.Println("skipping (value not present in params):", path)
			continue
		}

		segs := strings.SplitN(bParam[1], ".", 2)

		fieldName := "value"
		keyName := segs[0]
		if len(segs) == 2 {
			fieldName = segs[1]
		}

		fields, ok := vaultValues[keyName]
		if !ok {
			fields = map[string]interface{}{}
			vaultValues[keyName] = fields
		}

		fields[fieldName] = value.Value()
	}

	logical := vaultClient.Logical()

	for param, fields := range vaultValues {
		path := cmd.paramPath(param)

		if cmd.DryRun {
			log.Println("would write", param, "to path", path, "in vault")
			continue
		}

		log.Println("writing", param, "to path", path, "in vault")

		_, err := logical.Write(path, fields)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cmd *Command) paramPath(param string) string {
	if cmd.isTeamParam(param) {
		return path.Join(cmd.PathPrefix, cmd.TeamName, param)
	} else {
		return path.Join(cmd.PathPrefix, cmd.TeamName, cmd.PipelineName, param)
	}
}

func (cmd *Command) isTeamParam(param string) bool {
	for _, p := range cmd.TeamParams {
		if param == p {
			return true
		}
	}

	return false
}

func checkOrphaned(path string, a yaml.Node, b yaml.Node) {
	if a == nil {
		strB, ok := b.Value().(string)
		if ok {
			if interpolationRegex.MatchString(strB) {
				log.Fatalln("cannot correlate", path, strB, "to original value")
			}
		}
	}
}

func main() {
	cmd := &Command{}

	parser := flags.NewParser(cmd, flags.Default)
	parser.NamespaceDelimiter = "-"
	args, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}

	err = cmd.Execute(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
