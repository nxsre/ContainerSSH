package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	jsoniter "github.com/json-iterator/go"
	"go.containerssh.io/libcontainerssh"
	"gvisor.dev/gvisor/pkg/sentry/seccheck/points/points_go_proto"
	"gvisor.dev/gvisor/pkg/sentry/seccheck/sinks/remote/wire"
	"io"
	"os"
)

var assetTag = GetSerialNumber()

// 判断所给路径文件/文件夹是否存在
func Exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

// 判断所给路径是否为文件夹
func IsDir(path string) bool {
	s, err := os.Stat(path)
	if err != nil {
		return false
	}
	return s.IsDir()
}

// 判断所给路径是否为文件
func IsFile(path string) bool {
	return !IsDir(path)
}

// https://github.com/google/gvisor/tree/master/pkg/sentry/seccheck/sinks/remote
// https://github.com/google/gvisor/blob/master/pkg/sentry/seccheck/README.md
// https://github.com/google/gvisor/blob/master/g3doc/user_guide/quick_start/oci.md
// https://github.com/google/gvisor/blob/master/g3doc/user_guide/filesystem.md
// https://github.com/google/gvisor/blob/master/pkg/sentry/strace/linux64_arm64.go  syscall idmap
func main() {

	fmt.Println("启动 ssh server")
	libcontainerssh.Main()
}

type msgHandle struct {
	writer io.WriteCloser
	stream *jsoniter.Stream
}

//	func (m msgHandle) run() chan bool {
//		ticker := time.NewTicker(1 * time.Second)
//		stopChan := make(chan bool)
//
//		go func(ticker *time.Ticker) {
//			defer ticker.Stop()
//			for {
//				select {
//				case <-ticker.C:
//					m.stream.Flush()
//				case stop := <-stopChan:
//					if stop {
//						fmt.Println("Ticker Stop! Channel must be closed")
//						return
//					}
//				}
//			}
//		}(ticker)
//
//		return stopChan
//	}
func (m msgHandle) writeObj(val AuiditMsg) (int, error) {
	jb, err := jsoniter.Marshal(val)
	if err != nil {
		return 0, err
	}
	return m.writer.Write(append(jb, byte('\n')))
}

type AuiditMsg struct {
	AssetTag string      `json:"asset_tag"`
	Type     string      `json:"type"`
	Data     interface{} `json:"data"`
}

func (m msgHandle) Message(raw []byte, hdr wire.Header, payload []byte) error {
	//log.Println(points_go_proto.MessageType(hdr.MessageType).String())
	switch points_go_proto.MessageType(hdr.MessageType) {
	case points_go_proto.MessageType_MESSAGE_CONTAINER_START:
		msg := points_go_proto.Start{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_CONTAINER_START, string(jb))
	case points_go_proto.MessageType_MESSAGE_SENTRY_CLONE:
		msg := points_go_proto.Clone{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SENTRY_CLONE, string(jb))
	case points_go_proto.MessageType_MESSAGE_SENTRY_TASK_EXIT:
		msg := points_go_proto.TaskExit{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SENTRY_TASK_EXIT, string(jb))
	case points_go_proto.MessageType_MESSAGE_SYSCALL_OPEN:
		msg := points_go_proto.Open{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SYSCALL_OPEN, string(jb))
	case points_go_proto.MessageType_MESSAGE_SYSCALL_RAW:
		msg := points_go_proto.Syscall{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SYSCALL_RAW, string(jb))
	//case points_go_proto.MessageType_MESSAGE_SYSCALL_READ:
	//	msg := points_go_proto.Read{}
	//	err := proto.Unmarshal(payload, &msg)
	//	if err != nil {
	//		return err
	//	}
	//	jb, err := jsoniter.Marshal(msg)
	//	if err != nil {
	//		return err
	//	}
	//	log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SYSCALL_READ, string(jb))
	//case points_go_proto.MessageType_MESSAGE_SYSCALL_WRITE:
	//	msg := points_go_proto.Write{}
	//	err := proto.Unmarshal(payload, &msg)
	//	if err != nil {
	//		return err
	//	}
	//	jb, err := jsoniter.Marshal(msg)
	//	if err != nil {
	//		return err
	//	}
	//	log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SYSCALL_WRITE, string(jb))
	case points_go_proto.MessageType_MESSAGE_SENTRY_EXIT_NOTIFY_PARENT:
		msg := points_go_proto.ExitNotifyParentInfo{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SENTRY_EXIT_NOTIFY_PARENT, string(jb))
	case points_go_proto.MessageType_MESSAGE_SENTRY_EXEC:
		msg := points_go_proto.ExecveInfo{}
		err := proto.Unmarshal(payload, &msg)
		if err != nil {
			return err
		}
		m.writeObj(AuiditMsg{assetTag, points_go_proto.MessageType(hdr.MessageType).String(), msg})
		//jb, err := jsoniter.Marshal(msg)
		//if err != nil {
		//	return err
		//}
		//log.Printf("%s ：：： %+v", points_go_proto.MessageType_MESSAGE_SENTRY_EXEC, string(jb))
	default:
		//log.Println("==========", points_go_proto.MessageType(hdr.MessageType).String())
	}
	return nil
}

// Version returns what wire version of the protocol is supported.
func (m msgHandle) Version() uint32 {
	return 1
}

// Close closes the handler.
func (m msgHandle) Close() {
	return
}
