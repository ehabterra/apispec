package main

import (
	"fmt"
	"regexp"
)

func main() {
	pattern := "^\\*github\\.com/labstack/echo(/v\\d+)?\\.(Echo|Group)$"
	text := "*github.com/labstack/echo/v4.Group"
	matched, _ := regexp.MatchString(pattern, text)
	fmt.Println("Pattern:", pattern)
	fmt.Println("Text:", text)
	fmt.Println("Matched:", matched)

	// Test other variations
	texts := []string{
		"*github.com/labstack/echo/v4.Group",
		"*github.com/labstack/echo/v4.Echo",
		"*github.com/labstack/echo.Echo",
		"*github.com/labstack/echo.Group",
	}

	for _, t := range texts {
		matched, _ := regexp.MatchString(pattern, t)
		fmt.Printf("Text: %s -> Matched: %v\n", t, matched)
	}
}
