package imagectx

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

var (
	loadedFontOnce sync.Once
	loadedFace     font.Face
	loadedFontFaceMu sync.RWMutex
	loadedFontPath string
)

// GetDefaultFontFace returns a thread-safe font.Face for rendering text, preferring CJK fonts.
func GetDefaultFontFace(customFontPath string, sizePoints float64) (font.Face, error) {
	loadedFontFaceMu.RLock()
	if loadedFace != nil && (customFontPath == "" || customFontPath == loadedFontPath) {
		face := loadedFace
		loadedFontFaceMu.RUnlock()
		return face, nil
	}
	loadedFontFaceMu.RUnlock()

	loadedFontFaceMu.Lock()
	defer loadedFontFaceMu.Unlock()

	if loadedFace != nil && (customFontPath == "" || customFontPath == loadedFontPath) {
		return loadedFace, nil
	}

	if sizePoints <= 0 {
		sizePoints = 14.0
	}

	candidates := getSystemFontCandidates(customFontPath)
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		parsedFont, err := opentype.Parse(data)
		if err != nil {
			// Might be a TTC collection or unsupported table, try parse collection if possible
			parsedCollection, collErr := opentype.ParseCollection(data)
			if collErr == nil && parsedCollection.NumFonts() > 0 {
				parsedFont, err = parsedCollection.Font(0)
			}
		}

		if err == nil && parsedFont != nil {
			face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
				Size:    sizePoints,
				DPI:     72,
				Hinting: font.HintingFull,
			})
			if err == nil && face != nil {
				loadedFace = face
				loadedFontPath = path
				return loadedFace, nil
			}
		}
	}

	// Fallback to basicfont if no opentype font is available
	loadedFace = basicfont.Face7x13
	loadedFontPath = "basicfont:7x13"
	return loadedFace, nil
}

func getSystemFontCandidates(customPath string) []string {
	var candidates []string
	if customPath != "" {
		candidates = append(candidates, customPath)
	}

	switch runtime.GOOS {
	case "windows":
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		fontsDir := filepath.Join(winDir, "Fonts")
		candidates = append(candidates,
			filepath.Join(fontsDir, "msyh.ttc"),    // Microsoft YaHei
			filepath.Join(fontsDir, "msyhl.ttc"),   // Microsoft YaHei Light
			filepath.Join(fontsDir, "simhei.ttf"),   // SimHei
			filepath.Join(fontsDir, "simsun.ttc"),   // SimSun
			filepath.Join(fontsDir, "arialuni.ttf"), // Arial Unicode
			filepath.Join(fontsDir, "segoeui.ttf"),  // Segoe UI
			filepath.Join(fontsDir, "arial.ttf"),    // Arial
		)
	case "darwin":
		candidates = append(candidates,
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
		)
	default: // Linux / Android / BSD
		candidates = append(candidates,
			"/system/fonts/NotoSansCJK-Regular.ttc", // Android
			"/system/fonts/DroidSansFallback.ttf",  // Android
			"/system/fonts/Roboto-Regular.ttf",     // Android
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		)
	}

	return candidates
}

// ResetFontFaceForTest allows unit tests to reset the font face.
func ResetFontFaceForTest() {
	loadedFontFaceMu.Lock()
	defer loadedFontFaceMu.Unlock()
	loadedFace = nil
	loadedFontPath = ""
}
