package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

type Options struct {
	DryRun   bool
	Verbose  bool
	Force    bool
	Excludes []string
	Logfile  string
}

func main() {
	// Parametreleri ve flag’leri oku
	opts := Options{}
	var excludeList, logfile string
	pflag.BoolVar(&opts.DryRun, "dry-run", false, "Sadece neleri sileceğini göster")
	pflag.BoolVar(&opts.Verbose, "verbose", false, "Silinenleri detaylı göster")
	pflag.BoolVar(&opts.Force, "force", false, "Kilitli dosyalar için process kill uygula")
	pflag.StringVar(&excludeList, "exclude", "", "Hariç tutulacak pattern (virgülle ayır)")
	pflag.StringVar(&logfile, "logfile", "mf.log", "Log dosyası")
	pflag.Parse()

	opts.Logfile = logfile
	if excludeList != "" {
		opts.Excludes = strings.Split(excludeList, ",")
	}

	args := pflag.Args()
	if len(args) < 1 {
		fmt.Println("Kullanım: mf <hedef-yol> [--dry-run] [--verbose] [--force] [--exclude \"*.log,*.tmp\"] [--logfile <dosya>]")
		os.Exit(1)
	}
	target := args[0]

	// Kritik sistem dizinlerinden korun!
	if IsDangerousPath(target) {
		fmt.Printf("UYARI: %s silinemez (sistem dizini)\n", target)
		os.Exit(2)
	}

	// Logger’ı başlat
	StartLogger(opts.Logfile)
	defer StopLogger()

	err := Rimraf(target, opts)
	if err != nil {
		fmt.Printf("Hata: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("İşlem tamamlandı.")
}
