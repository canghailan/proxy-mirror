package main

import (
	"archive/zip"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"time"
	"flag"
	"strings"
	"net"
)

var port = flag.Int("port", 80, "port listening")
func main() {
	flag.Parse()
	log.Printf("listen %d\n", *port)

	http.HandleFunc("/mirror/", Mirror)
	http.HandleFunc("/git-zip/", GitZip)
	e := http.ListenAndServe(":" + strconv.Itoa(*port), nil)
	if e != nil {
		log.Fatal(e)
	}
}

func Mirror(w http.ResponseWriter, req *http.Request) {
	uri := req.URL.RequestURI()[len("/mirror/"):]
	httpUrl := HttpUrl(uri)
	log.Println(httpUrl)

	cookieJar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: cookieJar,
	}

	resp, e := client.Get(httpUrl)
	if e != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	io.Copy(w, resp.Body)
}

func GitZip(w http.ResponseWriter, req *http.Request) {
	uri := req.URL.RequestURI()[len("/git-zip/"):]
	httpUrl := HttpUrl(uri)
	log.Println(httpUrl)

	temp := MD5(httpUrl + "?" + Rand())
	repo := path.Base(uri)
	if path.Ext(uri) == ".git" {
		repo = repo[:len(repo)-len(".git")]
	}
	dir := filepath.Join(temp, repo)

	log.Printf("git clone %s %s\n", httpUrl, dir)
	cmd := exec.Command("git", "clone", httpUrl, dir)
	out, e := cmd.CombinedOutput()
	if e != nil {
		log.Println(e)
		http.Error(w, string(out), http.StatusInternalServerError)
		return
	}
	log.Println(string(out))

	w.Header().Set("content-type", "application/zip")
	w.Header().Set("content-disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", repo))
	if e = Zip(dir, w); e != nil {
		log.Println(e)
	}
	if e = os.RemoveAll(temp); e != nil {
		log.Println(e)
	}
}

func MD5(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return hex.EncodeToString(h.Sum(nil))
}

func Rand() string {
	i, e := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if e != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return i.String()
}

func Zip(dir string, w io.Writer) error {
	z := zip.NewWriter(w)
	e := filepath.Walk(dir, func(p string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		dst, e := z.Create(filepath.ToSlash(rel))
		if e != nil {
			return e
		}
		src, e := os.Open(p)
		if e != nil {
			return e
		}
		defer src.Close()
		_, e = io.Copy(dst, src)
		return e
	})
	if e != nil {
		return e
	}
	if e = z.Flush(); e != nil {
		return e
	}
	if e = z.Close(); e != nil {
		return e
	}
	return nil
}

func HttpUrl(uri string) string {
	host := strings.SplitN(uri, "/", 2)[0]
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "443")
	}
	c, e := net.DialTimeout("tcp", host, time.Duration(10) * time.Second)
	defer c.Close()
	if e != nil {
		return "http://" + uri
	} else {
		return "https://" + uri
	}
}