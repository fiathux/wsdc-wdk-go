package uuid

/* RFC 4122 UUID Implement
 * Fiathux Su
 * 2015 version 0.1
 */

import(
  "fmt"
  "net"
  "time"
  "encoding/binary"
  "strings"
  "encoding/hex"
  "crypto/rand"
  "crypto/md5"
  "crypto/sha1"
)

type UUID_t [16]byte //the UUID type

//UUID 3/5 namespaces
type UUID_NS UUID_t
var NAMESPACE_DNS = UUID_NS{0x6b,0xa7,0xb8,0x10,0x9d,0xad,0x11,0xd1,0x80,0xb4,0x00,0xc0,0x4f,0xd4,0x30,0xc8}
var NAMESPACE_URL = UUID_NS{0x6b,0xa7,0xb8,0x11,0x9d,0xad,0x11,0xd1,0x80,0xb4,0x00,0xc0,0x4f,0xd4,0x30,0xc8}
var NAMESPACE_OID = UUID_NS{0x6b,0xa7,0xb8,0x12,0x9d,0xad,0x11,0xd1,0x80,0xb4,0x00,0xc0,0x4f,0xd4,0x30,0xc8}
var NAMESPACE_X500 = UUID_NS{0x6b,0xa7,0xb8,0x14,0x9d,0xad,0x11,0xd1,0x80,0xb4,0x00,0xc0,0x4f,0xd4,0x30,0xc8}
//Null UUID
var UUID_NULL = UUID_t{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}

var GROUPS_UUID_B = []int{4,2,2,2,6}

var fristMAC [6]byte
var offline = true

func init() {
  ifList,err := net.Interfaces();
  if err != nil {
    return;
  }
  for _,if_t := range ifList {
    if len(if_t.HardwareAddr) == 6 {
      copy(fristMAC[0:6],if_t.HardwareAddr[0:6])
      offline = false
      return
    }
  }
}

func genTimestemp(buf []byte) {
  var tmsw [8]byte
  t := time.Now().UTC().UnixNano() / 100 + 122192928000000000 //Unix to Gregorian
  binary.BigEndian.PutUint64(tmsw[0:8],uint64(t))
  copy(buf[0:4],tmsw[4:8]) //Low time
  copy(buf[4:6],tmsw[2:4]) //Mid time
  copy(buf[6:8],tmsw[0:2]) //Hi time
}

func genVariants(buf []byte,version byte) {
  buf[6] = version<<4 | (buf[6] & 0x0f) //(M) Version
  buf[8] = 0x80 | (buf[8] & 0x3f)       //(N) RFC 4122
}

//UUID version 1
func UUID1() UUID_t {
  if offline {
    return UUID1r()
  }
  var rst UUID_t
  genTimestemp(rst[0:8])
  rand.Read(rst[8:10])
  copy(rst[10:16],fristMAC[0:6])
  genVariants(rst[0:16],1)
  return rst;
}

//UUID version 1 without network address
func UUID1r() UUID_t {
  var rst UUID_t
  genTimestemp(rst[0:8])
  rand.Read(rst[8:16])
  genVariants(rst[0:16],1)
  return rst;
}

//UUID version 3
func UUID3(ns UUID_NS,name []byte) UUID_t {
  var rst UUID_t
  reqstr := make([]byte,len(ns)+len(name))
  copy(reqstr[copy(reqstr,ns[:]):],name)
  pem := md5.Sum(reqstr)
  copy(rst[0:16],pem[0:16])
  genVariants(rst[0:16],3)
  return rst;
}

//UUID version 4
func UUID4() UUID_t {
  var rst UUID_t
  rand.Read(rst[0:16])
  genVariants(rst[0:16],4)
  return rst;
}

//UUID version 5
func UUID5(ns UUID_NS,name []byte) UUID_t {
  var rst UUID_t
  reqstr := make([]byte,len(ns)+len(name))
  copy(reqstr[copy(reqstr,ns[:]):],name)
  pem := sha1.Sum(reqstr)
  copy(rst[0:16],pem[0:16])
  genVariants(rst[0:16],5)
  return rst;
}

//Null UUID
func UUIDNull() UUID_t {
  return UUID_NULL
}

//UUID from string
func UUIDFrom(uuid_string string) (UUID_t,bool) {
  spid := strings.Split(uuid_string,"-")
  if len(spid) != 5 || len(spid[0]) != 8 || len(spid[1]) != 4 || len(spid[2]) != 4 ||
  len(spid[3]) != 4 || len(spid[4]) != 12 {
    return UUIDNull(),false
  }
  rst := UUID_t{}
  fillStart := 0
  for i:=0;i<5;i++ {
    clip,err := hex.DecodeString(spid[i])
    if err!=nil {
      return UUIDNull(),false
    }
    copy(rst[fillStart:fillStart+GROUPS_UUID_B[i]],clip)
    fillStart+=GROUPS_UUID_B[i]
  }
  return rst,true
}

//UUID to string
func (uid UUID_t) String() string {
  return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
  uid[0],uid[1],uid[2],uid[3],uid[4],uid[5],uid[6],uid[7],uid[8],uid[9],
  uid[10],uid[11],uid[12],uid[13],uid[14],uid[15])
}

func (uid UUID_t) IsNull() bool {
  return uid == UUID_NULL
}
