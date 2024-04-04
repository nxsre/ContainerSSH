package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/juju/ratelimit"
	"github.com/nxsre/tcpshaper/bandwidth"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/artyom/autoflags"
	"github.com/pkg/sftp"
)

func main() {
	args := struct {
		Addr string `flag:"addr,address to listen"`
		Auth string `flag:"auth,path to authorized_keys file"`
		PK   string `flag:"hostkey,path to private host key"`
	}{
		Addr: "localhost:2022",
		Auth: "authorized_keys",
		PK:   "id_rsa",
	}
	autoflags.Define(&args)
	flag.Parse()
	if err := run(args.Addr, args.Auth, args.PK); err != nil {
		log.Fatal(err)
	}
}

func run(addr, keysFile, pkFile string) error {
	config := &ssh.ServerConfig{}
	if err := addHostKey(config, pkFile); err != nil {
		return err
	}
	pkeyAuthFunc, err := authChecker(keysFile)
	if err != nil {
		return err
	}
	config.PublicKeyCallback = pkeyAuthFunc
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		//conn = bandwidth.NewRateLimitedConn(context.Background(), readLimiter, writeLimiter, conn)
		if err != nil {
			return err
		}
		go func(conn net.Conn) {
			if err := serveConn(conn, config); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func serveConn(conn net.Conn, config *ssh.ServerConfig) error {
	defer log.Println("serveConn finished")
	defer conn.Close()
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return err
	}
	defer sconn.Close()
	_ = sconn // TODO: check Permissions
	go ssh.DiscardRequests(reqs)

	// 可以根据用户名设置限速规则
	log.Println(sconn.User())
	// 两种方法
	// 1. 用 "github.com/nxsre/tcpshaper/bandwidth"
	// 2. 用 "github.com/juju/ratelimit", ratelimit 不能动态调速率
	// 设置读写限速规则
	readConfig := bandwidth.NewRateConfig(2560000, 512000000)
	readLimiter := bandwidth.NewBandwidthLimiter(readConfig)
	writeConfig := bandwidth.NewRateConfig(2560000, 512000000)
	writeLimiter := bandwidth.NewBandwidthLimiter(writeConfig)
	_, _ = readLimiter, writeLimiter
	go func() {
		time.Sleep(30 * time.Second)
		log.Println("调速度")
		readConfig.SetLimit(2560000 * 2)
		writeConfig.SetLimit(2560000 * 2)
	}()

	// 512000000 为初始（突发）不限速容量，用完后速率为 2560000
	bucket := ratelimit.NewBucketWithQuantum(1*time.Second, 512000000, 2560000)
	_ = bucket

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()

		// channel 限速
		//channel = NewRateLimitedChannel(context.Background(), bucket, channel)
		channel = bandwidth.NewRateLimitedChannel(context.Background(), readLimiter, writeLimiter, channel)

		if err != nil {
			return err
		}
		go func(sshCh ssh.Channel, in <-chan *ssh.Request) {
			quitSignal := make(chan int, 1)
			defer log.Println("channel/requets handler finished")
			for req := range in {
				var ok bool
				switch {
				case req.Type == "exec":
					ok = true
					cmd := string(req.Payload[4:])
					args := strings.Split(cmd, " ")
					if strings.HasPrefix(args[0], "scp") {
						scp := NewSCP(channel, afero.NewOsFs(), log.WithField("module", "scp"))
						go scp.Main(args[1:], quitSignal)
						req.Reply(true, nil)
						<-quitSignal
						continue
					}
				case req.Type == "pty-req":
					req.Reply(true, nil)
					continue
				case req.Type == "shell":
					ok = true
					go func() {
						// defer sconn.Close()      // XXX(?)
						defer sshCh.Close()      // SSH_MSG_CHANNEL_CLOSE
						defer sshCh.CloseWrite() // SSH_MSG_CHANNEL_EOF
						defer sshCh.SendRequest("odv@openssh.com", false, nil)
						switch err := serveTerminal(sshCh); err {
						case nil:
							sshCh.SendRequest("exit-status", false, ssh.Marshal(&exitStatusMsg{0}))
						default:
							sshCh.SendRequest("exit-status", false, ssh.Marshal(&exitStatusMsg{1}))
						}
					}()
				case req.Type == "subsystem" && string(req.Payload[4:]) == "sftp":
					ok = true
					go func() {
						defer sshCh.Close() // SSH_MSG_CHANNEL_CLOSE
						sftpServer, err := sftp.NewServer(sshCh, sftp.ReadOnly())
						if err != nil {
							return
						}
						_ = sftpServer.Serve()
					}()
				}
				req.Reply(ok, nil)
				if ok {
					break
				}
			}
			for req := range in {
				req.Reply(false, nil)
			}
		}(channel, requests)
	}
	return nil
}

func serveTerminal(rw io.ReadWriter) error {
	log.Println("serveTerminal started")
	defer log.Println("serveTerminal finished")
	term := terminal.NewTerminal(rw, "> ")
	for {
		line, err := term.ReadLine()
		switch err {
		case nil:
		case io.EOF:
			return nil
		default:
			return err
		}
		log.Println("line read:", line)
		if _, err := fmt.Fprintf(term, "You said: %q\n", line); err != nil {
			return err
		}
	}
}

func authChecker(name string) (func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error), error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	type keyMeta struct {
		key  ssh.PublicKey
		opts map[string]string
	}
	var pkeys []keyMeta
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		pk, _, opts, _, err := ssh.ParseAuthorizedKey(sc.Bytes())
		if err != nil {
			return nil, err
		}
		pkeys = append(pkeys, keyMeta{key: pk, opts: splitOpts(opts)})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		keyBytes := key.Marshal()
		for _, k := range pkeys {
			if bytes.Equal(keyBytes, k.key.Marshal()) {
				return &ssh.Permissions{
					Extensions: k.opts,
				}, nil
			}
		}
		return nil, fmt.Errorf("no keys matched")
	}, nil
}

func splitOpts(opts []string) map[string]string {
	if len(opts) == 0 {
		return nil
	}
	m := make(map[string]string, len(opts))
	for _, s := range opts {
		ss := strings.SplitN(s, "=", 2)
		switch len(ss) {
		case 1:
			m[s] = ""
		case 2:
			m[ss[0]] = ss[1]
		}
	}
	return m
}

func addHostKey(config *ssh.ServerConfig, keyFile string) error {
	privateBytes, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return err
	}
	config.AddHostKey(private)
	return nil
}

type exitStatusMsg struct {
	Status uint32
}
