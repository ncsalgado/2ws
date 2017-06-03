/*
    This program (2ws) is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, version 3 of the License.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.

*/
package main
/*
TODOs:
1. Test tar.gz:
   1. Empty dirs to create
   2. Removed dirs to create
   3. File created inside dir with 2 level deep
   4. Create Files with spaces in root and in subdirs 
   5. Create Files with \t and \n in root and in subdirs
2. Implement Events
3. Create Config Option to Create Local and Replica Backups
4. Update IUL and IUR base on IAL and IAR
5. Always running to detect Local and Local Replica chnages
6. Detect Replica changes
7. Sync by http

*/
import (
	"bufio"
//	"bytes"
	"hash/crc32"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/spf13/viper"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"strconv"
	"syscall"
	"time"
)

const cAppName = "2ws"
const cIdxName = 0
const cIdxSize = 1
const cIdxiNode = 2
const cIdxmodTime = 3
const cIdxMod = 4
const cIdxUid = 5
const cIdxGid = 6
const cIdxCRC = 7
const cIdxStatus = 8

type configConSyncTp struct {
	_ParentConfigCon *configConTp
	_Id            string
	_IULFilePath   string
	_IALFilePath   string
	_DILFilePath   string
	_CALFilePath   string
	_IURFilePath   string
	_IARFilePath   string
	_DIRFilePath   string
	_CARFilePath   string
	_RSLFilePath   string // Files/Folders from Local to Replica
	_RSRFilePath   string // Files/Folders from Replica to Local
	_LocalBakRoot  string
	_DIRReplicaFilePath string
	_CARReplicaFilePath string
	_RSLReplicaFilePath string
	_ReplicaBakRoot string
	_FirstTime     bool
	SyncSubfolders string
	ReplicaRoot    string
	ReplicaEvents  string
	LocalRoot      string
	LocalEvents    string
	PathsList      []string
}

type configConTp struct {
	_ParentConfig  *configTp
	_ReplicaUserDir string
	_ReplicaWorkDirPath string
	_ReplicaAppFilePath string
	_ReplicaConfigFilePath string
	_ReplicaHomeDirPath string
	_ReplicaUserHost string
	ReplicaConnection string
	NetworkConnectionsToReplicaList     []string
	NetworkConnectionsAllowedDisallowed string
	SyncsList []configConSyncTp
}

type configLstConTp []configConTp

type configTp struct {
	_TimeStamp      string
	_WorkDirPath string
	_HomeDirPath string
	ConnectionsList configLstConTp
}


func CheckErrorPanic(iErr error, iMsgErr string){
	if iErr != nil { // Handle errors
		panic(fmt.Errorf("Fatal error %s in %s\n", iErr, iMsgErr))
	}
}

func hash_file_crc32(filePath string, polynomial uint32) string {
	/*
http://www.mrwaggel.be/post/generate-crc32-hash-of-a-file-in-go/
*/
	var returnCRC32String string
	if polynomial == 0 {
		polynomial = crc32.Castagnoli
	}
	file, err := os.Open(filePath)
	CheckErrorPanic(err, "hash_file_crc32, opening file " + filePath)
	defer file.Close()
	tablePolynomial := crc32.MakeTable(polynomial)
	hash := crc32.New(tablePolynomial)
	_, err = io.Copy(hash, file)
	CheckErrorPanic(err, "hash_file_crc32 in file " + filePath)
	hashInBytes := hash.Sum(nil)[:]
	returnCRC32String = hex.EncodeToString(hashInBytes)
	return returnCRC32String
}

func readConfig(iConfigFilePath string) (vConfigDef configTp) {
	vConfig := viper.New()
	vConfig.SetConfigType("hcl")
	vConfig.SetConfigName(cAppName)             // name of config file (without extension)
	if iConfigFilePath != "" {
		vConfig.AddConfigPath(iConfigFilePath) // call multiple times to add many search paths
	} else {
		vConfig.AddConfigPath("$HOME/." + cAppName) // call multiple times to add many search paths
		vConfig.AddConfigPath(".")                  // optionally look for config in the working directory
	}
	// Read Config File
	err := vConfig.ReadInConfig()
	CheckErrorPanic(err, fmt.Sprintf("Reading config file %s", cAppName))
	// Parse Config File
	err = vConfig.Unmarshal(&vConfigDef)
	CheckErrorPanic(err, fmt.Sprintf("Decoding into struct"))
	// Working folder no local $HOME
	if usr, err := user.Current(); err != nil {
		vConfigDef._HomeDirPath = "."
	} else {
		vConfigDef._HomeDirPath = usr.HomeDir
	}
	vConfigDef._WorkDirPath = filepath.Join(vConfigDef._HomeDirPath, "."+cAppName)
	// Name for backup folder
	vConfigDef._TimeStamp = fmt.Sprint(time.Now().UTC().UnixNano())
	// Create Connections Id's and full file's path for each sync
	crc32q := crc32.MakeTable(0xD5828281)
	for iC := 0; iC < len(vConfigDef.ConnectionsList); iC++ {
		// Store parent
		vConfigDef.ConnectionsList[iC]._ParentConfig = &vConfigDef

		if (vConfigDef.ConnectionsList[iC].ReplicaConnection == "") {
			vConfigDef.ConnectionsList[iC]._ReplicaWorkDirPath = vConfigDef._WorkDirPath
		} else {
			// Extract User@Host (vUserHost) and Host "Home" Dir (vHostPath)
			var vPosColon int
			var vUserHost, vHostPath string

			vPosColon = strings.Index(vConfigDef.ConnectionsList[iC].ReplicaConnection, ":")
			if vPosColon == -1 {
				vUserHost = vConfigDef.ConnectionsList[iC].ReplicaConnection
				vHostPath = "~"
			} else {
				vUserHost = vConfigDef.ConnectionsList[iC].ReplicaConnection[:vPosColon]
				vHostPath = vConfigDef.ConnectionsList[iC].ReplicaConnection[vPosColon+1:]
			}
			// Set Replica working dir
			vConfigDef.ConnectionsList[iC]._ReplicaUserHost = vUserHost
			vConfigDef.ConnectionsList[iC]._ReplicaHomeDirPath = vHostPath
			vConfigDef.ConnectionsList[iC]._ReplicaWorkDirPath = filepath.Join(vHostPath, "."+cAppName)
			vConfigDef.ConnectionsList[iC]._ReplicaAppFilePath = filepath.Join(vHostPath, cAppName)
			vConfigDef.ConnectionsList[iC]._ReplicaConfigFilePath = filepath.Join(vConfigDef.ConnectionsList[iC]._ReplicaWorkDirPath, cAppName+".hcl")
		}
		for iCS := 0; iCS < len(vConfigDef.ConnectionsList[iC].SyncsList); iCS++ {
			// Simplify left side
			vConSync := &vConfigDef.ConnectionsList[iC].SyncsList[iCS]
			// Store parent
			vConSync._ParentConfigCon = &vConfigDef.ConnectionsList[iC]
			// To track changes, Hash name of Connection + ReplicaRoot + LocalRoot
			vConSync._Id = fmt.Sprintf("%08x", crc32.Checksum([]byte(vConfigDef.ConnectionsList[iC].ReplicaConnection + vConSync.ReplicaRoot + vConSync.LocalRoot), crc32q))
			if err := os.Mkdir(filepath.Join(vConfigDef._WorkDirPath, vConSync._Id), 0755); ! os.IsExist(err) {
				CheckErrorPanic(err, fmt.Sprintf("Creating working dir %s", vConSync._Id))
			}
			// Se Deu erro->Pasta não existia por isso 1ª vez 
			vConSync._FirstTime = err != nil
			// Backup folder
			//err := os.Mkdir(filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, vConfigDef._TimeStamp), 0755)
			//CheckErrorPanic(err, fmt.Sprintf("Creating backup dir %s", filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, vConfigDef._TimeStamp)))

			vConSync._LocalBakRoot =   filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, vConfigDef._TimeStamp)
			vConSync._ReplicaBakRoot = filepath.Join(vConSync._ParentConfigCon._ReplicaWorkDirPath, vConSync._Id, vConfigDef._TimeStamp + "R")

			vConSync._IULFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "iul.txt")
			vConSync._IURFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "iur.txt")
			vConSync._IARFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "iar.txt")
			vConSync._IALFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "ial.txt")
			vConSync._DILFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "dil.txt")
			vConSync._DIRFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "dir.txt")
			vConSync._DIRReplicaFilePath = filepath.Join(vConSync._ParentConfigCon._ReplicaWorkDirPath, vConSync._Id, "dir.txt")
			vConSync._CALFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "cal.sh")
			vConSync._CARFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "car.sh")
			vConSync._CARReplicaFilePath = filepath.Join(vConSync._ParentConfigCon._ReplicaWorkDirPath, vConSync._Id, "car.sh")
			vConSync._RSLFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "rsl.txt")
			vConSync._RSLReplicaFilePath = filepath.Join(vConSync._ParentConfigCon._ReplicaWorkDirPath, vConSync._Id, "rsl.txt")
			vConSync._RSRFilePath = filepath.Join(vConfigDef._WorkDirPath, vConSync._Id, "rsr.txt")
			
		}//for iCS
	}
	return
}

func criaNovoSeNaoExisteIUL(iConfigCon configConSyncTp) {
	os.OpenFile(iConfigCon._IULFilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
}

func criaNovoSeNaoExisteIUR(iConfigCon configConSyncTp) {
	os.OpenFile(iConfigCon._IURFilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
}

func criaIUL(iConfigCon configConSyncTp) {
	criaIA_Local(iConfigCon._IULFilePath, iConfigCon.LocalRoot, iConfigCon.PathsList, false)
}

func criaIUR(iConfigCon configConSyncTp) {
	criaIA_Local(iConfigCon._IURFilePath, iConfigCon.ReplicaRoot, iConfigCon.PathsList, true)
}

func criaIA_Local(iIA_LocalFilePath, iLocalRoot string, iLstPaths []string, iDoFileCRC bool) {
	vFp, err := os.Create(iIA_LocalFilePath)
	if err != nil {
		fmt.Println("Could not open file: " + iIA_LocalFilePath)
	}
	for _, vPath := range iLstPaths {
		vInventoryPath := filepath.Join(iLocalRoot, vPath)
		// Check paths
		if _, err := os.Stat(vInventoryPath); os.IsNotExist(err) {
			CheckErrorPanic(err, fmt.Sprintf("dir %s", vInventoryPath))
		}
		filepath.Walk(vInventoryPath,
			func(iFilePath string, iInfo os.FileInfo, err error) error {
				var vDirChar, vFileCRC string
				if iInfo.IsDir() {
					vDirChar = "/"
				} else {
					vDirChar = ""
					if iDoFileCRC{
						vFileCRC = hash_file_crc32(iFilePath, 0)
					}
				}
				vRelPath, _ := filepath.Rel(iLocalRoot, iFilePath)
				vFileInfo, _ := os.Stat(iFilePath)
				vStat, _ := vFileInfo.Sys().(*syscall.Stat_t)
				// Path+File/Folder_Name \t Size \t iNode \t ModTime \t mod \t Owner \t Group \t CRC \t Status \n
				vFp.WriteString(fmt.Sprintf("%+q\t%v\t%v\t%v\t%#o\t%v\t%v\t%s\t\n",
					vRelPath+vDirChar,
					iInfo.Size(),
					vStat.Ino,
					iInfo.ModTime().Format(time.RFC3339),
					iInfo.Mode() & os.ModePerm,
					iInfo.Sys().(*syscall.Stat_t).Uid,
					iInfo.Sys().(*syscall.Stat_t).Gid,
					vFileCRC,
				))
				return nil
			})
	}
	vFp.Close()
}

func criaIAL(iConfigCon configConSyncTp) {
	criaIA_Local(iConfigCon._IALFilePath, iConfigCon.LocalRoot, iConfigCon.PathsList, false)
}

func criaIAR(iConfigCon configConSyncTp) {
	criaIA_Local(iConfigCon._IARFilePath, iConfigCon.ReplicaRoot, iConfigCon.PathsList, true)
}

func dif2Files(iFileNamePathOld, iFileNamePathNew, iFileNamePathDif string) {
	vFO, _ := os.Open(iFileNamePathOld); defer vFO.Close()
	vFN, _ := os.Open(iFileNamePathNew); defer vFN.Close()
	vFD, _ := os.Create(iFileNamePathDif); defer vFD.Close()

	vScannerFO := bufio.NewScanner(vFO)
	vScannerFN := bufio.NewScanner(vFN)

	vTerminou := false

	vScannerFO.Scan()
	vScannerFN.Scan()

	for !vTerminou {
// fmt.Println(vScannerFO.Text(), vScannerFN.Text())
		if vScannerFO.Text() == vScannerFN.Text() {
			// Lines are equals
			if vScannerFO.Text() == "" && vScannerFN.Text() == "" {
			// both lines are empty -> end of both files
				vTerminou = true
			} else {
			// Next lines
				vScannerFO.Scan()
				vScannerFN.Scan()
			}
		} else {
			vLineSplitO := strings.Split(vScannerFO.Text(), "\t")
			vLineSplitN := strings.Split(vScannerFN.Text(), "\t")
			vFileNameO := vLineSplitO[cIdxName]
			vFileNameN := vLineSplitN[cIdxName]
			if vFileNameO == vFileNameN {
				// Same name in both files
				var vStatus string
				if (vLineSplitO[cIdxSize] != vLineSplitN[cIdxSize]) {
					// File has different size => is different
					vStatus = "A"
				}else if (strings.Join(vLineSplitO[cIdxiNode:cIdxmodTime], "\t") != strings.Join(vLineSplitN[cIdxiNode:cIdxmodTime], "\t")){
					// File may have changed
					vStatus = "A"
				}else{
					// File has different attributes
					vStatus = "a"
				}
				vLineSplitN[cIdxStatus] = vStatus
				vFD.WriteString(fmt.Sprintln(strings.Join(vLineSplitN, "\t")))
				vScannerFO.Scan()
				vScannerFN.Scan()
			} else if ((vFileNameO != "") && (vFileNameO < vFileNameN)) || vFileNameN == "" {
				vLineSplitO[cIdxStatus] = "D"
				vFD.WriteString(fmt.Sprintln(strings.Join(vLineSplitO, "\t")))
				//fmt.Println("apagado")
				vScannerFO.Scan()
			} else {
				vLineSplitN[cIdxStatus] = "N"
				vFD.WriteString(fmt.Sprintln(strings.Join(vLineSplitN, "\t")))
				//fmt.Println("novo")
				vScannerFN.Scan()
			}
		}
	}
}

func fazDIL(iConfigCon configConSyncTp) {
	dif2Files(iConfigCon._IULFilePath, iConfigCon._IALFilePath, iConfigCon._DILFilePath)
}

func fazDIR(iConfigCon configConSyncTp) {
	dif2Files(iConfigCon._IURFilePath, iConfigCon._IARFilePath, iConfigCon._DIRFilePath)
}

func BackupCmd(iFileName, iRoot, iBakRoot, ilstBakPath string) (oCmd string, olstBakPath string) {
	vFilePathNameFrom := filepath.Join(iRoot, iFileName)
	vFilePathNameTo := filepath.Join(iBakRoot, iFileName)
	// Backup Path
	vFilePathBak := filepath.Dir(vFilePathNameTo)
	if (vFilePathBak > ilstBakPath){
		oCmd = fmt.Sprintf("mkdir -p \"%s\"\n", vFilePathBak)
		olstBakPath = vFilePathBak
	} else {
		oCmd = ""
		olstBakPath = ilstBakPath
	}
	oCmd += fmt.Sprintf("cp \"%s\" \"%s\"\n", vFilePathNameFrom, vFilePathNameTo)
	return
}

func DiffDILandDIR(iConfigCon configConSyncTp){
	vFdiL, _ := os.Open(iConfigCon._DILFilePath);   defer vFdiL.Close()
	vFdiR, _ := os.Open(iConfigCon._DIRFilePath);   defer vFdiR.Close()
	vFcaL, _ := os.Create(iConfigCon._CALFilePath); defer vFcaL.Close()
	vFcaR, _ := os.Create(iConfigCon._CARFilePath); defer vFcaR.Close()
	vFrsL, _ := os.Create(iConfigCon._RSLFilePath); defer vFrsL.Close()
	vFrsR, _ := os.Create(iConfigCon._RSRFilePath); defer vFrsR.Close()
	var vlstLocalBakPath, vlstReplicaBakPath string
	var vLstDelDirL, vLstDelDirR []string
	
	vScannerFdiL := bufio.NewScanner(vFdiL)
	vScannerFdiR := bufio.NewScanner(vFdiR)
	
	vTerminou := false
	
	vScannerFdiL.Scan()
	vScannerFdiR.Scan()
	
	for !vTerminou {
		vLineSplitdiL := strings.Split(vScannerFdiL.Text(), "\t")
		vLineSplitdiR := strings.Split(vScannerFdiR.Text(), "\t")
		vFileNameL, _ := strconv.Unquote(vLineSplitdiL[cIdxName])
		vFileNameR, _ := strconv.Unquote(vLineSplitdiR[cIdxName])
		vFileNamePathL := filepath.Join(iConfigCon.LocalRoot,   vFileNameL)
		vFileNamePathR := filepath.Join(iConfigCon.ReplicaRoot, vFileNameR)

		if (vFileNameL == "") && (vFileNameR == "") {
			// both lines are empty -> end of both files
			vTerminou = true
		} else if (vFileNameL == vFileNameR) {
			vStatusFileL := vLineSplitdiL[cIdxStatus]
			vStatusFileR := vLineSplitdiR[cIdxStatus]

			vFnHandleConflict := func (
				// Handle files conflict
			){
				fnNewConfFileNamePath := func(iFileNamePath, iHostname string) (oNewFileNamePath string){
					// Generate new name to rename file in conflict
					vExt := filepath.Ext(iFileNamePath)
					vBaseTrim := strings.TrimSuffix(filepath.Base(iFileNamePath), vExt)
					if iHostname == "" {
						iHostname, _ = os.Hostname()
					}
					oNewFileNamePath = filepath.Join(filepath.Dir(iFileNamePath), vBaseTrim+" (Confl "+iHostname +")"+vExt)
					return
				}
				
				var vCmd string
				// Backup Local;
				vCmd, vlstLocalBakPath = BackupCmd(vFileNameL, iConfigCon.LocalRoot, iConfigCon._LocalBakRoot, vlstLocalBakPath)
				vFcaL.WriteString(vCmd)
				
				// Rename Local NAME (Confl <PC>).EXT;
				vNewFileNamePath := fnNewConfFileNamePath(vFileNamePathL, "")
				vFcaL.WriteString(fmt.Sprintf("mv \"%s\" \"%s\"\n", vFileNamePathL, vNewFileNamePath))
				
				// RCopy Local to Replica
				vFrsL.WriteString(fmt.Sprintf("%s\n", fnNewConfFileNamePath(vFileNameL, "")))
				
				// Backup Replica;
				vCmd, vlstReplicaBakPath = BackupCmd(vFileNameR, iConfigCon.ReplicaRoot, iConfigCon._ReplicaBakRoot, vlstReplicaBakPath)
				vFcaR.WriteString(vCmd)
				
				// Rename Replica NAME (Confl REPLICA).EXT;
				vNewFileNamePath = fnNewConfFileNamePath(vFileNamePathR, "REPLIC")
				vFcaR.WriteString(fmt.Sprintf("mv \"%s\" \"%s\"\n", vFileNamePathR, vNewFileNamePath))
				
				// RCopy Replica to Local
				vFrsR.WriteString(fmt.Sprintf("%s\n", fnNewConfFileNamePath(vFileNameR, "REPLIC")))
			}
			
			if (vFileNameL[len(vFileNameL)-1] == '/') {
				// DIR
				switch{
				case (vStatusFileL == "a") && (vStatusFileR == "a"):
					// Local and Replica Attribs are different => Copy Replica attribs to Local attribs
					vFcaL.WriteString(fmt.Sprintf("chmod \"%s\" \"%s\"; chown %s:%s \"%s\"\n",
						vLineSplitdiR[cIdxMod], vFileNamePathL,
						vLineSplitdiR[cIdxGid], vLineSplitdiR[cIdxUid], vFileNamePathL))
				case ((vStatusFileL == "a") && (vStatusFileR == "D")):
					// Replica deleted dir => Backup Local; Delete Local
					vLstDelDirL = append(vLstDelDirL, fmt.Sprintf("rmdir \"%s\"\n", vFileNamePathL))
				case ((vStatusFileL == "D") && (vStatusFileR == "a")):
					// Local deleted file => Backup Replica; Delete Replica
					vLstDelDirR = append(vLstDelDirR, fmt.Sprintf("rmdir \"%s\"\n", vFileNamePathR))
				}
			} else {
				// FILE
				switch {
				case (vStatusFileL == "a") && (vStatusFileR == "a"):
					// Local and Replica Attribs are different => Copy Replica attribs to Local attribs
					vFcaL.WriteString(fmt.Sprintf("chmod \"%s\" \"%s\"; chown %s:%s \"%s\"; touch -m -d %s \"%s\"\n",
						vLineSplitdiR[cIdxMod], vFileNamePathL,
						vLineSplitdiR[cIdxGid], vLineSplitdiR[cIdxUid], vFileNamePathL,
						vLineSplitdiR[cIdxmodTime], vFileNamePathL))
				case ((vStatusFileL == "a") && ((vStatusFileR == "A") || (vStatusFileR == "N"))):
					// Replica has new or different file => Backup Local; Copy Replica to Local
					var vCmd string
					vCmd, vlstLocalBakPath = BackupCmd(vFileNameL, iConfigCon.LocalRoot, iConfigCon._LocalBakRoot, vlstLocalBakPath)
					vFcaL.WriteString(vCmd)
					vFrsR.WriteString(fmt.Sprintf("%s\n", vFileNameR))
				case (((vStatusFileL == "A") || (vStatusFileL == "N")) && (vStatusFileR == "a")):
					// Local has new or different file => Backup Replica; Copy Local to Replica
					var vCmd string
					vCmd, vlstReplicaBakPath = BackupCmd(vFileNameR, iConfigCon.ReplicaRoot, iConfigCon._ReplicaBakRoot, vlstReplicaBakPath)
					vFcaR.WriteString(vCmd)
					vFrsL.WriteString(fmt.Sprintf("%s\n", vFileNameL))
				case ((vStatusFileL == "a") && (vStatusFileR == "D")):
					// Replica deleted file => Backup Local; Delete Local
					var vCmd string
					vCmd, vlstLocalBakPath = BackupCmd(vFileNameL, iConfigCon.LocalRoot, iConfigCon._LocalBakRoot, vlstLocalBakPath)
					vFcaL.WriteString(vCmd)
					vFcaL.WriteString(fmt.Sprintf("rm \"%s\"\n", vFileNamePathL))
				case ((vStatusFileL == "D") && (vStatusFileR == "a")):
					// Local deleted file => Backup Replica; Delete Replica
					var vCmd string
					vCmd, vlstReplicaBakPath = BackupCmd(vFileNameR, iConfigCon.ReplicaRoot, iConfigCon._ReplicaBakRoot, vlstReplicaBakPath)
					vFcaR.WriteString(vCmd)
					vFcaR.WriteString(fmt.Sprintf("rm \"%s\"\n", vFileNamePathR))
				case ((vStatusFileL == "N") && (vStatusFileR == "D")) || ((vStatusFileL == "D") && (vStatusFileR == "N")):
					// One is New the Other Deleted => Conflict
					vFnHandleConflict()
				case ((vStatusFileL == "A") && (vStatusFileR == "D")) || ((vStatusFileL == "D") && (vStatusFileR == "A")):
					// One as Changed Other Deleted => Conflict
					vFnHandleConflict()
				case (((vStatusFileL == "A") || (vStatusFileL == "N")) && ((vStatusFileR == "N") || (vStatusFileR == "A"))) && (vLineSplitdiL[cIdxSize] != vLineSplitdiR[cIdxSize]):
					// One is New the other as changed and Sizes are different => Conflict
					vFnHandleConflict()
				case (((vStatusFileL == "A") || (vStatusFileL == "N")) && ((vStatusFileR == "N") || (vStatusFileR == "A"))) && (vLineSplitdiL[cIdxSize] == vLineSplitdiR[cIdxSize]):
					// One is New the other as changed and Sizes are equal try CRC32 => Conflict
					vLocalFileCRC := hash_file_crc32(vFileNamePathL, 0)
					if vLocalFileCRC != vLineSplitdiR[cIdxCRC] {
						// CRC are different => Files are different => Conflit
						vFnHandleConflict()
					} else if vLineSplitdiL[cIdxmodTime] > vLineSplitdiR[cIdxmodTime] {
						// CRC are equal files are the same maybe Attrbs are different
						// Local and Replica Attribs are different => Copy Local attribs to Replica attribs
						vFcaR.WriteString(fmt.Sprintf("chmod %s \"%s\"; chown %s:%s \"%s\"; touch -m -d %s \"%s\"\n",
							vLineSplitdiL[cIdxMod], vFileNamePathR,
							vLineSplitdiL[cIdxGid], vLineSplitdiL[cIdxUid], vFileNamePathR,
							vLineSplitdiL[cIdxmodTime], vFileNamePathR))
					} else {
						// Local and Replica Attribs are different => Copy Replica attribs to Local attribs
						vFcaL.WriteString(fmt.Sprintf("chmod %s \"%s\"; chown %s:%s \"%s\"; touch -m -d %s \"%s\"\n",
							vLineSplitdiR[cIdxMod], vFileNamePathL,
							vLineSplitdiR[cIdxGid], vLineSplitdiR[cIdxUid], vFileNamePathL,
							vLineSplitdiR[cIdxmodTime], vFileNamePathL))
						
					}
				}//switch
			}//else (if (vFileNameL[len(vFileNameL)-1] == '/'))
			vScannerFdiL.Scan()
			vScannerFdiR.Scan()
		} else if ((vFileNameL != "") && (vFileNameL < vFileNameR)) || vFileNameR == "" {
			// Name only in Local
			vStatusFileL := vLineSplitdiL[cIdxStatus]
			vFileNamePathR := filepath.Join(iConfigCon.ReplicaRoot, vFileNameL)
			if (vFileNameL[len(vFileNameL)-1] == '/') {
				// DIR
				switch{
				case (vStatusFileL == "a"):
					// Local Attribs are different => Copy Local attribs to Replica attribs
					vFcaR.WriteString(fmt.Sprintf("chmod \"%s\" \"%s\"; chown %s:%s \"%s\"\n",
						vLineSplitdiL[cIdxMod], vFileNamePathR,
						vLineSplitdiL[cIdxGid], vLineSplitdiL[cIdxUid], vFileNamePathR))
				case (vStatusFileL == "N"):
					// Local as new or different Dir => Backup Replica; Copy Local to Replica
					vFcaR.WriteString(fmt.Sprintf("mkdir \"%s\"\n", vFileNamePathR))
				case (vStatusFileL == "D"):
					// Local deleted file => Backup Replica; Delete Replica
					vLstDelDirR = append(vLstDelDirR, fmt.Sprintf("rmdir \"%s\"\n", vFileNamePathR))
				}//switch
			} else {
				// FILES
				switch {
				case (vStatusFileL == "a"):
					// Attribs changed in Local => Change in Replica
					vFcaR.WriteString(fmt.Sprintf("chmod %s \"%s\"; chown %s:%s \"%s\"; touch -m -d %s \"%s\"\n",
						vLineSplitdiL[cIdxMod], vFileNamePathR,
						vLineSplitdiL[cIdxGid], vLineSplitdiL[cIdxUid], vFileNamePathR,
						vLineSplitdiL[cIdxmodTime], vFileNamePathR))
				case (vStatusFileL == "A"):
					// Local as new or different file => Backup Replica; Copy Local to Replica
					var vCmd string
					vCmd, vlstReplicaBakPath = BackupCmd(vFileNameL, iConfigCon.ReplicaRoot, iConfigCon._ReplicaBakRoot, vlstReplicaBakPath)
					vFcaR.WriteString(vCmd)
					vFrsL.WriteString(fmt.Sprintf("%s\n", vFileNameL))
				case (vStatusFileL == "N"):
					// Local as new or different file => Backup Replica; Copy Local to Replica
					vFrsL.WriteString(fmt.Sprintf("%s\n", vFileNameL))
				case (vStatusFileL == "D"):
					// Local deleted file => Backup Replica; Delete Replica
					var vCmd string
					vCmd, vlstReplicaBakPath = BackupCmd(vFileNameL, iConfigCon.ReplicaRoot, iConfigCon._ReplicaBakRoot, vlstReplicaBakPath)
					vFcaR.WriteString(vCmd)
					vFcaR.WriteString(fmt.Sprintf("rm \"%s\"\n", vFileNamePathR))
				}
			}
			vScannerFdiL.Scan()
		} else {
			// Name only in Replica
			vStatusFileR := vLineSplitdiR[cIdxStatus]
			vFileNamePathL := filepath.Join(iConfigCon.LocalRoot,   vFileNameR)
			if (vFileNameR[len(vFileNameR)-1] == '/') {
				// DIR
				switch{
				case (vStatusFileR == "a"):
					// Replica Attribs are different => Copy Replica attribs to Local attribs
					vFcaL.WriteString(fmt.Sprintf("chmod \"%s\" \"%s\"; chown %s:%s \"%s\"\n",
						vLineSplitdiR[cIdxMod], vFileNamePathL,
						vLineSplitdiR[cIdxGid], vLineSplitdiR[cIdxUid], vFileNamePathL))
				case (vStatusFileR == "N"):
					// Local as new or different Dir => Backup Replica; Copy Local to Replica
					vFcaL.WriteString(fmt.Sprintf("mkdir \"%s\"\n", vFileNamePathL))
				case (vStatusFileR == "D"):
					// Local deleted file => Backup Replica; Delete Replica
					vLstDelDirL = append(vLstDelDirL, fmt.Sprintf("rmdir \"%s\"\n", vFileNamePathL))
				}
			} else {
				// FILES
				switch {
				case (vStatusFileR == "a"):
					// Attribs changed in Replica => Change in Local
					vFcaL.WriteString(fmt.Sprintf("chmod %s \"%s\"; chown %s:%s \"%s\"; touch -m -d %s \"%s\"\n",
						vLineSplitdiR[cIdxMod], vFileNamePathL,
						vLineSplitdiR[cIdxGid], vLineSplitdiR[cIdxUid], vFileNamePathL,
						vLineSplitdiR[cIdxmodTime], vFileNamePathL))
				case (vStatusFileR == "A"):
					// Replica as new or different file => Backup Local; Copy Replica to Local
					var vCmd string
					vCmd, vlstLocalBakPath = BackupCmd(vFileNameR, iConfigCon.LocalRoot, iConfigCon._LocalBakRoot, vlstLocalBakPath)
					vFcaL.WriteString(vCmd)
					vFrsR.WriteString(fmt.Sprintf("%s\n", vFileNameR))
				case (vStatusFileR == "N"):
					// Replica as new or different file => Backup Local; Copy Replica to Local
					vFrsR.WriteString(fmt.Sprintf("%s\n", vFileNameR))
				case (vStatusFileR == "D"):
					// Replica deleted file => Backup Local; Delete Local
					var vCmd string
					vCmd, vlstLocalBakPath = BackupCmd(vFileNameR, iConfigCon.LocalRoot, iConfigCon._LocalBakRoot, vlstLocalBakPath)
					vFcaL.WriteString(vCmd)
					vFcaL.WriteString(fmt.Sprintf("rm \"%s\"\n", vFileNamePathL))
				}
			}
			vScannerFdiR.Scan()
		}
	}//for
	for i:=len(vLstDelDirL)-1; i>=0; i-- {
		vFcaL.WriteString(vLstDelDirL[i])
	}
	for i:=len(vLstDelDirR)-1; i>=0; i-- {
		vFcaR.WriteString(vLstDelDirR[i])
	}
}

func ExecOsCmd(iCmd string, iLstArgs ...string) (err error) {
	cmd := exec.Command(iCmd, iLstArgs...)
//	cmd.Stdin = strings.NewReader("some input")
//	var out bytes.Buffer
//	cmd.Stdout = &out
//	fmt.Printf("%s %s\n", iCmd, iLstArgs)
	err = cmd.Run()
	if err != nil {
		CheckErrorPanic(err, fmt.Sprintf("executar comando %s(%s)", iCmd, iLstArgs))
	}
	return
}

func twows(iConfigFilePath, iReplicaConnectionToSync string){
	vConfigDef := readConfig(iConfigFilePath)
	for _, vConfCon := range vConfigDef.ConnectionsList {
		if (iReplicaConnectionToSync == "") || (vConfCon.ReplicaConnection == iReplicaConnectionToSync) {
//fmt.Println(vConfCon.ReplicaConnection)
			for _, vConfConSync := range vConfCon.SyncsList {
//fmt.Println(vConfConSync._Id)
				criaIAL(vConfConSync)
				criaNovoSeNaoExisteIUL(vConfConSync)
				fazDIL(vConfConSync)
				if (vConfCon.ReplicaConnection == "") {
					criaIAR(vConfConSync)
					criaNovoSeNaoExisteIUR(vConfConSync)
					fazDIR(vConfConSync)
					DiffDILandDIR(vConfConSync)
					ExecOsCmd("/bin/sh", vConfConSync._CARFilePath)
					// Send files to Replica
					ExecOsCmd("/usr/bin/rsync", fmt.Sprintf("--files-from=%s", vConfConSync._RSLFilePath), "-R",
						vConfConSync.LocalRoot, vConfConSync.ReplicaRoot)
					criaIUR(vConfConSync)
					ExecOsCmd("/bin/sh", vConfConSync._CALFilePath)
					// Receive files from Replica
					ExecOsCmd("/usr/bin/rsync", fmt.Sprintf("--files-from=%s", vConfConSync._RSRFilePath), "-R",
						vConfConSync.ReplicaRoot, vConfConSync.LocalRoot)
					criaIUL(vConfConSync)
				} else {
/*
TODO:
cannot copy 2ws to replica because it could not be the same type of processor: //extrai o path para a aplicação
gera o ficheiro config para o servidor
*/
					// Send Config File to Replica
					ExecOsCmd("/usr/bin/rsync", cAppName+".hcl", vConfCon.ReplicaConnection)
					// Create IAR on Replica
					ExecOsCmd("/usr/bin/ssh",   vConfCon._ReplicaUserHost, fmt.Sprintf("%s -r %s -o IAR", vConfCon._ReplicaAppFilePath, vConfCon.ReplicaConnection))
					// Create DIR on Replica
					ExecOsCmd("/usr/bin/ssh",   vConfCon._ReplicaUserHost, fmt.Sprintf("%s -r %s -o DIR", vConfCon._ReplicaAppFilePath, vConfCon.ReplicaConnection))
					// Get created DIR from Host to Local
					ExecOsCmd("/usr/bin/rsync", vConfCon._ReplicaUserHost+":"+vConfConSync._DIRReplicaFilePath, vConfConSync._DIRFilePath)
					// Analyse Differences
					DiffDILandDIR(vConfConSync)
					// Send CAR.sh to Replica
					ExecOsCmd("/usr/bin/rsync", vConfConSync._CARFilePath, vConfCon._ReplicaUserHost+":"+vConfConSync._CARReplicaFilePath)
					// Exec remote CAR.sh
					ExecOsCmd("/usr/bin/ssh",   vConfCon._ReplicaUserHost, fmt.Sprintf("/bin/sh %s", vConfConSync._CARReplicaFilePath))
					// Send files to Replica
					ExecOsCmd("/usr/bin/rsync", fmt.Sprintf("--files-from=%s", vConfConSync._RSLFilePath), "-R",
						vConfConSync.LocalRoot, vConfCon._ReplicaUserHost+":"+vConfConSync.ReplicaRoot)
					// Create IUR on Host
					ExecOsCmd("/usr/bin/ssh",   vConfCon._ReplicaUserHost, fmt.Sprintf("%s -r %s -o IUR", vConfCon._ReplicaAppFilePath, vConfCon.ReplicaConnection))
					// Exec CAL.sh
					ExecOsCmd("/bin/sh", vConfConSync._CALFilePath)
					// Receive files from Replica
					ExecOsCmd("/usr/bin/rsync", fmt.Sprintf("--files-from=%s", vConfConSync._RSRFilePath), "-R",
						vConfCon._ReplicaUserHost+":"+vConfConSync.ReplicaRoot, vConfConSync.LocalRoot)
					// Create IUL
					criaIUL(vConfConSync)
				}
			}//for
		}//if vConfCon.ReplicaConnection == iReplicaConnectionToSync
	}//for
}


func main() {
	var vOp, vConfigFilePath, vReplicaConnectionToSync string
    flag.StringVar(&vOp, "o", "", "Options available: IAL, DIL, IAR, DIR, DIF, IUL, IUR")
    flag.StringVar(&vConfigFilePath, "c", "", "Config file path")
    flag.StringVar(&vReplicaConnectionToSync, "r", "", "Replica connection to sync")

	flag.Parse()
	
	if vOp == "" {
		twows(vConfigFilePath, vReplicaConnectionToSync)
	} else {
		vConfigDef := readConfig(vConfigFilePath)
		for _, vConfCon := range vConfigDef.ConnectionsList {
			if (vReplicaConnectionToSync == "") || (vConfCon.ReplicaConnection == vReplicaConnectionToSync) {
				for _, vConfConSync := range vConfCon.SyncsList {
					switch vOp{
					case "IAL": criaIAL(vConfConSync)
					case "DIL":
						criaNovoSeNaoExisteIUL(vConfConSync)
						fazDIL(vConfConSync)
					case "IAR": criaIAR(vConfConSync)
					case "DIR":
						criaNovoSeNaoExisteIUR(vConfConSync)
						fazDIR(vConfConSync)
					case "DIF": DiffDILandDIR(vConfConSync)
					case "CAR": ExecOsCmd("/bin/sh", vConfConSync._CARFilePath)
					case "CAL": ExecOsCmd("/bin/sh", vConfConSync._CALFilePath)
					case "IUL": criaIUL(vConfConSync)
					case "IUR": criaIUR(vConfConSync)
					}//switch
				}//for
			}//if vConfCon.ReplicaConnection == vReplicaConnectionToSync
		}//for
	}
}
