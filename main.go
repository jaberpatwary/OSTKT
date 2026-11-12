package main

import (
	"flag"
	"log"

	"ticket-system/internal/adapter/excel"
	"ticket-system/internal/app"
	"ticket-system/internal/config"
)

func main() {
	sampleExcel := flag.Bool("sample-excel", false, "create a sample employee workbook")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}
	if *sampleExcel {
		if err := excel.WriteSampleWorkbook(cfg.ExcelPath); err != nil {
			log.Fatalf("create sample workbook: %v", err)
		}
		log.Printf("sample workbook created: %s", cfg.ExcelPath)
		return
	}
	if err := app.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
