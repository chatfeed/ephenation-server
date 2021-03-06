// Copyright 2012 The Ephenation Authors
//
// This file is part of Ephenation.
//
// Ephenation is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3.
//
// Ephenation is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Ephenation.  If not, see <http://www.gnu.org/licenses/>.
//

package main

import (
	"chunkdb"
	"encoding/json"
	"ephenationdb"
	"flag"
	"fmt"
	"github.com/robfig/goconfig/config"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"superchunk"
	"time"
	"traffic"
)

var (
	tflag               = flag.Bool("dotest", false, "Run the test suite and then terminate.")
	procFlag            = flag.Int("p", 2, "Number of processes to use")
	ipPort              = flag.String("i", ":57862", "IP port to listen on")
	logFileName         = flag.String("log", "worldserver.log", "Log file name")
	allowTestUser       = flag.Bool("testuser", false, "Allow connection of testusers without password named 'testX', where X is a number")
	verboseFlag         = flag.Int("v", 0, "Verbose, Higher number gives more")
	cpuprofile          = flag.String("cpuprofile", "", "write cpu profile to file")
	convertChunkFiles   = flag.Bool("convertChunk", false, "Convert chunk files to new file format")
	welcomeMsgFile      = flag.String("welcome", "welcome.txt", "The file that is displayed at login")
	logOnStdout         = flag.Bool("s", false, "Send log file to standard otput")
	inhibitCreateChunks = flag.Bool("nocreate", false, "Only load modified chunks, and save no changes")
	configFileName      = flag.String("configfile", "config.ini", "General configuration file")
	dumpsql             = flag.Bool("dumpsql", false, "Create a dump of the complete SQL DB, and then exit")

	trafficStatistics = traffic.New()
	superChunkManager = superchunk.New(CnfgSuperChunkFolder)
	encryptionSalt    = ""
)

func main() {
	flag.Parse()
	cnfg, err := config.ReadDefault(*configFileName)
	if err != nil {
		log.Println("Fail to find", *configFileName, err)
		return
	}
	if cnfg.HasSection("sql") {
		server, err := cnfg.String("sql", "DatabaseServer")
		if err != nil {
			log.Println(*configFileName, "DatabaseServer:", err)
			return
		}
		name, err := cnfg.String("sql", "DatabaseName")
		if err != nil {
			log.Println(*configFileName, "DatabaseName:", err)
			return
		}
		login, err := cnfg.String("sql", "DatabaseLogin")
		if err != nil {
			log.Println(*configFileName, "DatabaseLogin:", err)
			return
		}
		pwd, err := cnfg.String("sql", "DatabasePassword")
		if err != nil {
			log.Println(*configFileName, "DatabasePassword:", err)
			return
		}
		ephenationdb.SetConnection(server, name, login, pwd)
	} else {
		log.Println("Config file", configFileName, "missing, or no section 'sql'")
	}
	if encryptionSalt, err = cnfg.String("login", "salt"); err != nil {
		encryptionSalt = "" // Effectively no salt
	}
	if *convertChunkFiles {
		ConvertFiles()
		return
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile() // Also done from special command /shutdown
	}
	if !*logOnStdout {
		logFile, _ := os.OpenFile(*logFileName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		log.SetOutput(logFile)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	if *dumpsql {
		DumpSQL()
		os.Exit(0)
	}
	if *tflag {
		DoTest()
		return
	}
	log.Printf("Pheenadv world server\n")
	if *verboseFlag > 0 {
		log.Printf("Verbose flag set to %d\n", *verboseFlag)
	}
	if *inhibitCreateChunks {
		log.Println("No chunks will be created or saved")
	}
	runtime.GOMAXPROCS(*procFlag)
	rand.Seed(time.Now().UnixNano())
	host, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	log.Printf("Start world server on %s\n", host)
	if *allowTestUser {
		log.Printf("Testusers without password allowed\n")
	}
	err = SetupListenForClients_WLuBlWLqWLa(*ipPort)
	if err != nil {
		log.Printf("%v, server abort\n", err)
		os.Exit(1)
	}
	go chunkdb.Poll_Bl() // Will terminate if there is no access to the SQL DB
	go ProcAutosave_RLu()
	go ProcPurgeOldChunks_WLw()
	go CatchSig()
	ManageMonsters_WLwWLuWLqWLmBlWLc() // Will not return
}

// Read all chunks, update them, and write them back again
// This function can also be used for convert from one chunk file format to another, with some tweaking.
func ConvertFiles() {
	dir, err := ioutil.ReadDir(CnfgChunkFolder)
	if err != nil {
		fmt.Printf("Failed to read . (%v)", err)
		return
	}
	var mod, unmod int
	for _, fi := range dir {
		fn := fi.Name()
		// fmt.Printf("%v ", fn)
		coords := strings.Split(fn, ",")
		if len(coords) != 3 {
			fmt.Printf("Skipping %v, bad file name for chunk\n", fn)
			continue
		}
		x, err := strconv.Atoi(coords[0])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		y, err := strconv.Atoi(coords[1])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		z, err := strconv.Atoi(coords[2])
		if err != nil {
			fmt.Printf("Chunk %v bad file name\n", fn)
			continue
		}
		c := chunkdb.CC{int32(x), int32(y), int32(z)}
		ch := dBFindChunkFromFS(c)
		if ch.flag&CHF_MODIFIED != 0 {
			mod++
		} else {
			unmod++
			name := DBChunkFileName(c)
			err = os.Remove(name)
			if err != nil {
				fmt.Printf("Failed to remove unmodified file %v, err %v\n", fn, err)
			}
		}
	}
	fmt.Printf("%d Modified, %d non modified\n", mod, unmod)
}

func DumpSQL() {
	db := ephenationdb.New()
	if db == nil {
		return
	}
	defer ephenationdb.Release(db)

	// Build a query for the avatar name sent as an argument
	// TODO: Assert that the avatar name is unique and on this server for the current user?
	query := "SELECT name,jsonstring,id,PositionX,PositionY,PositionZ,isFlying,isClimbing,isDead,DirHor,DirVert,AdminLevel,Level,Experience,HitPoints,Mana,Kills,HomeX,HomeY,HomeZ,ReviveX,ReviveY,ReviveZ,maxchunks,BlocksAdded,BlocksRemoved,TimeOnline,HeadType,BodyType,inventory,TScoreTotal,TScoreBalance,TScoreTime,TargetX,TargetY,TargetZ FROM avatars"
	stmt, err := db.Prepare(query)
	if err != nil {
		log.Println(err)
		return
	}

	// Execute statement
	err = stmt.Execute()
	if err != nil {
		log.Println(err)
		return
	}

	// Some helper variables
	var packedline string
	var uid uint32
	var packedInv []byte
	var terrScore, terrScoreBalance float64
	var terrScoreTimestamp uint32
	// Booleans doesn't work
	var flying, climbing, dead int
	var pl player
	stmt.BindResult(&pl.name, &packedline, &uid, &pl.coord.X, &pl.coord.Y, &pl.coord.Z, &flying, &climbing, &dead, &pl.dirHor, &pl.dirVert, &pl.adminLevel, &pl.level,
		&pl.exp, &pl.hitPoints, &pl.mana, &pl.numKill, &pl.homeSP.X, &pl.homeSP.Y, &pl.homeSP.Z, &pl.reviveSP.X, &pl.reviveSP.Y, &pl.reviveSP.Z, &pl.maxchunks,
		&pl.blockAdd, &pl.blockRem, &pl.timeOnline, &pl.head, &pl.body, &packedInv, &terrScore, &terrScoreBalance, &terrScoreTimestamp,
		&pl.targetCoor.X, &pl.targetCoor.Y, &pl.targetCoor.Z)

	for {
		eof, err := stmt.Fetch()
		if err != nil {
			log.Println(err)
			return
		}
		if eof {
			break
		}
		// Some post processing
		if flying == 1 {
			pl.flying = true
		}
		if climbing == 1 {
			pl.climbing = true
		}
		if dead == 1 {
			pl.dead = true
		}

		if pl.maxchunks == -1 {
			// This parameter was not initialized.
			pl.maxchunks = CnfgMaxOwnChunk
		}
		DumpSQLPlayer(&pl, packedline, packedInv)
	}
}

func DumpSQLPlayer(pl *player, packedline string, packedInv []byte) {

	// log.Println(pl.targetCoor)

	pl.logonTimer = time.Now()
	var err error

	if err = json.Unmarshal([]uint8(packedline), pl); err != nil {
		log.Printf("Unmarshal player %s: %v (%v)\n", pl.name, err, packedline)
		// TODO: This covers errors when updating the jsonstring, should be handled in a more approperiate way
		//return 0, false
	}

	// If there was data in the inventory "blob", unpack it.
	if len(packedInv) > 0 {
		err = pl.inventory.Unpack([]byte(packedInv))
		if err != nil {
			log.Println("Failed to unpack", err, packedInv)
		}
		// Save what can be saved, and remove unknown objects.
		pl.inventory.CleanUp()
	}
	if *verboseFlag > 1 {
		log.Println("Inventory unpacked", pl.inventory)
	}

	//fmt.Printf("Coord: (%v,%v,%v)\n", pl.coord.X, pl.coord.Y, pl.coord.Z )

	if pl.reviveSP.X == 0 && pl.reviveSP.Y == 0 && pl.reviveSP.Z == 0 {
		// Check if there is any spawn point defined.
		pl.reviveSP = pl.coord
		pl.homeSP = pl.coord
	}

	fmt.Printf("User: %#v\n", pl)
}
