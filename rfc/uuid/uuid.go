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
  "crypto/rand"
)


type UUID_t [16]byte //the UUID type

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
func GenUUID1() UUID_t {
  if offline {
    return GenUUID1f()
  }
  var rst UUID_t
  genTimestemp(rst[0:8])
  rand.Read(rst[8:10])
  copy(rst[10:16],fristMAC[0:6])
  genVariants(rst[0:16],1)
  return rst;
}

//UUID version 1 offline (with out MAC address)
func GenUUID1f() UUID_t {
  var rst UUID_t
  genTimestemp(rst[0:8])
  rand.Read(rst[8:16])
  genVariants(rst[0:16],1)
  return rst;
}

//UUID version 4
func GenUUID4() UUID_t {
  var rst UUID_t
  rand.Read(rst[0:16])
  genVariants(rst[0:16],4)
  return rst;
}

func NullUUID() UUID_t {
  return UUID_t{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
}

//UUID to string
func (uid UUID_t) String() string {
  return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
  uid[0],uid[1],uid[2],uid[3],uid[4],uid[5],uid[6],uid[7],uid[8],uid[9],
  uid[10],uid[11],uid[12],uid[13],uid[14],uid[15])
}

func (uid UUID_t) IsNull() bool {
  return uid == UUID_t{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
}
