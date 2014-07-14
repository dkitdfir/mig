/* Mozilla InvestiGator Console

Version: MPL 1.1/GPL 2.0/LGPL 2.1

The contents of this file are subject to the Mozilla Public License Version
1.1 (the "License"); you may not use this file except in compliance with
the License. You may obtain a copy of the License at
http://www.mozilla.org/MPL/

Software distributed under the License is distributed on an "AS IS" basis,
WITHOUT WARRANTY OF ANY KIND, either express or implied. See the License
for the specific language governing rights and limitations under the
License.

The Initial Developer of the Original Code is
Mozilla Corporation
Portions created by the Initial Developer are Copyright (C) 2014
the Initial Developer. All Rights Reserved.

Contributor(s):
Julien Vehent jvehent@mozilla.com [:ulfr]

Alternatively, the contents of this file may be used under the terms of
either the GNU General Public License Version 2 or later (the "GPL"), or
the GNU Lesser General Public License Version 2.1 or later (the "LGPL"),
in which case the provisions of the GPL or the LGPL are applicable instead
of those above. If you wish to allow use of your version of this file only
under the terms of either the GPL or the LGPL, and not to allow others to
use your version of this file under the terms of the MPL, indicate your
decision by deleting the provisions above and replace them with the notice
and other provisions required by the GPL or the LGPL. If you do not delete
the provisions above, a recipient may use your version of this file under
the terms of any one of the MPL, the GPL or the LGPL.
*/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mig"
	migdb "mig/database"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"

	"code.google.com/p/gcfg"
	"github.com/bobappleyard/readline"
	"github.com/jvehent/cljs"
)

type Context struct {
	API struct {
		URL string
	}
	HTTP struct {
		Client http.Client
	}
	Homedir string
	GPG     struct {
		Home, KeyID string
	}
}

var ctx Context

func main() {
	var err error
	fmt.Println("\x1b[32;1m" + `
## ##                                     _.---._     .---.
# # # /-\ ---||  |    /\         __...---' .---. '---'-.   '.
#   #|   | / ||  |   /--\    .-''__.--' _.'( | )'.  '.  '._ :
#   # \_/ ---| \_ \_/    \ .'__-'_ .--'' ._'---'_.-.  '.   '-'.
     ###                         ~ -._ -._''---. -.    '-._   '.
      # |\ |\    /---------|          ~ -.._ _ _ _ ..-_ '.  '-._''--.._
      # | \| \  / |- |__ | |                       -~ -._  '-.  -. '-._''--.._.--''.
     ###|  \  \/  ---__| | |                            ~ ~-.__     -._  '-.__   '. '.
          #####                                               ~~ ~---...__ _    ._ .' '.
          #      /\  --- /-\ |--|----                                    ~  ~--.....--~
          # ### /--\  | |   ||-\  //
          #####/    \ |  \_/ |  \//__
` + "\x1b[0m")

	ctx.Homedir = findHomedir()
	// command line options
	var config = flag.String("c", ctx.Homedir+"/.migconsole", "Load configuration from file")
	var api = flag.String("a", "undef", "API base url (ex: http://localhost:1664/api/v1/)")
	flag.Parse()

	if *api != "undef" {
		ctx.API.URL = *api
	} else {
		err := gcfg.ReadFileInto(&ctx, *config)
		if err != nil {
			panic(err)
		}
	}
	ctx.GPG.Home, err = findGPGHome(ctx)
	if err != nil {
		panic(err)
	}

	err = printStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nConnected to %s. Exit with \x1b[32;1mctrl+d\x1b[0m. Type \x1b[32;1mhelp\x1b[0m for help.\n", ctx.API.URL)
	for {
		// completion
		var symbols = []string{"action", "agent", "command", "help", "exit", "showcfg", "status",
			"search", "where", "and"}
		readline.Completer = func(query, ctx string) []string {
			var res []string
			for _, sym := range symbols {
				if strings.HasPrefix(sym, query) {
					res = append(res, sym)
				}
			}
			return res
		}

		input, err := readline.String("\x1b[32;1mmig>\x1b[0m ")
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("error: ", err)
			break
		}
		orders := strings.Split(input, " ")
		switch orders[0] {
		case "action":
			if len(orders) == 2 {
				if orders[1] == "new" {
					var a mig.Action
					err = actionLauncher(a, ctx)
				} else {
					err = actionReader(input, ctx)
				}
				if err != nil {
					log.Println(err)
				}
			} else {
				fmt.Println("error: 'action' order takes one argument; " +
					"either 'new' to enter launcher mode, or an action ID to enter reader mode.")
			}
		case "agent":
			err = agentReader(input, ctx)
			if err != nil {
				log.Println(err)
			}
		case "command":
			err = commandReader(input, ctx)
			if err != nil {
				log.Println(err)
			}
		case "exit":
			fmt.Printf("exit\n")
			goto exit
		case "help":
			fmt.Printf(`The following orders are available:
action <id|new>	enter action mode. if <id> is given, go to reader mode. if "new" is given, enter launcher mode.
command <id>	enter command reader mode for command <id>
help		show this help
exit		leave
search		perform a search. see "search help" for more information.
showcfg		display running configuration
status		display platform status: connected agents and latest actions
`)
		case "search":
			err = search(input, ctx)
			if err != nil {
				log.Println(err)
			}
		case "showcfg":
			fmt.Printf("homedir = %s\n[api]\n    url = %s\n[gpg]\n    home = %s\n    keyid = %s\n",
				ctx.API.URL, ctx.Homedir, ctx.GPG.Home, ctx.GPG.KeyID)
		case "status":
			err = printStatus(ctx)
			if err != nil {
				log.Println(err)
			}
		case "":
			break
		default:
			fmt.Printf("Unknown order '%s'\n", orders[0])
		}
		readline.AddHistory(input)
	}
exit:
	fmt.Printf(`
            .-._   _ _ _ _ _ _ _ _
 .-''-.__.-'Oo  '-' ' ' ' ' ' ' ' '-.
'.___ '    .   .--_'-' '-' '-' _'-' '._
 V: V 'vv-'   '_   '.       .'  _..' '.'.
   '=.____.=_.--'   :_.__.__:_   '.   : :
           (((____.-'        '-.  /   : :
                             (((-'\ .' /
                           _____..'  .'
                          '-._____.-'
              Gators are going back underwater.
`)
}

func findHomedir() string {
	if runtime.GOOS == "darwin" {
		return os.Getenv("HOME")
	} else {
		// find keyring in default location
		u, err := user.Current()
		if err != nil {
			panic(err)
		}
		return u.HomeDir
	}
}

// findGPGHome looks for the GnuPG home directory
func findGPGHome(ctx Context) (home string, err error) {
	if ctx.GPG.Home != "" {
		home = ctx.GPG.Home
	} else {
		var gnupghome string
		gnupghome = os.Getenv("GNUPGHOME")
		if gnupghome == "" {
			gnupghome = "/.gnupg"
		}
		home = ctx.Homedir + gnupghome
	}
	for _, loc := range [3]string{home, home + "/pubring.gpg", home + "/secring.gpg"} {
		_, err := os.Stat(loc)
		if err != nil {
			log.Fatalf("'%s' not found\n", loc)
		}
	}
	return
}

func getAPIResource(t string, ctx Context) (resource *cljs.Resource, err error) {
	resp, err := ctx.HTTP.Client.Get(t)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	resource = cljs.New("")
	err = json.Unmarshal(body, &resource)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("HTTP %d: %v (code %s)", resp.StatusCode, resource.Collection.Error.Message, resource.Collection.Error.Code)
		return
	}
	return
}

func printStatus(ctx Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("printStatus() -> %v", e)
		}
	}()
	targetURL := ctx.API.URL + "dashboard"
	st, err := getAPIResource(targetURL, ctx)
	if err != nil {
		panic(err)
	}
	agtout := make([]string, 3)
	agtout[0] = "Agents Summary:"
	actout := make([]string, 2)
	actout[0] = "Latest Actions:"
	actout[1] = "----    ID      ---- + ----         Name         ---- + ----    Date    ---- + ---- Investigators ----"
	for _, item := range st.Collection.Items {
		for _, data := range item.Data {
			switch data.Name {
			case "action":
				idstr, name, datestr, invs, err := actionPrintShort(data.Value)
				if err != nil {
					panic(err)
				}
				str := fmt.Sprintf("%s   %s   %s   %s", idstr, name, datestr, invs)
				actout = append(actout, str)
			case "active agents":
				agtout[1] = fmt.Sprintf("* %.0f agents have checked in during the last 5 minutes", data.Value)
			case "agents versions count":
				bData, err := json.Marshal(data.Value)
				if err != nil {
					panic(err)
				}
				var sum []migdb.AgentsSum
				err = json.Unmarshal(bData, &sum)
				if err != nil {
					panic(err)
				}
				for _, asum := range sum {
					s := fmt.Sprintf("* version %s: %.0f agent", asum.Version, asum.Count)
					if asum.Count > 1 {
						s += "s"
					}
					agtout = append(agtout, s)
				}
			case "agents started in the last 24 hours":
				agtout[2] = fmt.Sprintf("* %.0f agents (re)started in the last 24 hours", data.Value)
			}
		}
	}
	fmt.Println("\x1b[31;1m+------\x1b[0m")
	for _, s := range agtout {
		fmt.Println("\x1b[31;1m|\x1b[0m " + s)
	}
	fmt.Println("\x1b[31;1m|\x1b[0m")
	for _, s := range actout {
		fmt.Println("\x1b[31;1m|\x1b[0m " + s)
		if len(actout) < 2 {
			fmt.Println("\x1b[31;1m|\x1b[0m * None")
			break
		}
	}
	fmt.Println("\x1b[31;1m+------\x1b[0m")
	return
}