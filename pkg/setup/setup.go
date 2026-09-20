package setup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/boggycreek/lokol/pkg/model"
	"github.com/boggycreek/lokol/pkg/probe"
)

// Options controls setup behavior.
type Options struct {
	DownloadModel bool
	InstallLlama  bool
	SimulateVRAM  float64
}

// Result captures the outcome of the setup inspection and actions.
type Result struct {
	Hardware       *probe.HardwareProfile
	Recommendation model.Recommendation
	LlamaInstalled bool
	LlamaPath      string
	ModelInstalled bool
	ModelPath      string
	ServiceActive  bool
	Issues         []string
}

// Run performs environment verification, hardware probing, and optional dependency setup.
func Run(opts Options) (*Result, error) {
	fmt.Println("==================================================")
	fmt.Println("             lokol Environment Setup              ")
	fmt.Println("==================================================")
	fmt.Println()

	// 1. Hardware Probe
	fmt.Println("[1/4] Probing host hardware capabilities...")
	hw, err := probe.Detect()
	if err != nil {
		return nil, fmt.Errorf("failed probing hardware: %w", err)
	}

	if opts.SimulateVRAM > 0 {
		fmt.Printf("  [SIMULATION] Overriding detected VRAM with: %.2f GiB\n", opts.SimulateVRAM)
		hw.VRAMBytes = uint64(opts.SimulateVRAM * 1024 * 1024 * 1024)
		hw.HasNVIDIA = true
		hw.GPUName = fmt.Sprintf("Simulated GPU (%.1f GiB VRAM)", opts.SimulateVRAM)
	}

	rec := model.SelectOptimalModel(hw)
	res := &Result{
		Hardware:       hw,
		Recommendation: rec,
	}

	fmt.Printf("  Platform     : %s / %s (%d CPU cores)\n", hw.OS, hw.Arch, hw.CPUCores)
	fmt.Printf("  System RAM   : %s\n", hw.HumanRAM())
	if hw.GPUName != "" {
		fmt.Printf("  GPU Detected : %s (%s VRAM)\n", hw.GPUName, hw.HumanVRAM())
	} else {
		fmt.Println("  GPU Detected : None (CPU mode)")
	}
	fmt.Printf("  Hardware Tier: %s\n", rec.Tier)
	fmt.Printf("  Recommended  : %s\n", rec.ModelName)
	fmt.Printf("  KV Context   : %d tokens (%s)\n", rec.ContextLength, rec.KVCacheQuant)
	fmt.Println()

	// 2. llama / llama-server detection
	fmt.Println("[2/4] Checking inference engine dependencies...")
	llamaPath, llamaFound := findLlamaBinary()
	res.LlamaInstalled = llamaFound
	res.LlamaPath = llamaPath

	if llamaFound {
		fmt.Printf("  ✓ Found llama binary at: %s\n", llamaPath)
	} else {
		fmt.Println("  ✗ llama binary not found in PATH or ~/.local/bin/llama.")
		res.Issues = append(res.Issues, "llama binary missing (needed for local inference engine)")
	}

	// 3. Recommended model weight verification
	fmt.Println()
	fmt.Println("[3/4] Checking recommended model weights...")
	modelDir := filepath.Join(os.Getenv("HOME"), "models")
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		modelDir = filepath.Join(xdgData, "lokol", "models")
	}
	_ = os.MkdirAll(modelDir, 0755)

	targetModelPath := filepath.Join(modelDir, rec.HFFile)
	// Check if already in standard ~/models or XDG
	if altPath := filepath.Join(os.Getenv("HOME"), "models", rec.HFFile); fileExists(altPath) {
		targetModelPath = altPath
	}

	res.ModelPath = targetModelPath
	if fileExists(targetModelPath) {
		res.ModelInstalled = true
		fmt.Printf("  ✓ Model weights verified at: %s\n", targetModelPath)
	} else {
		fmt.Printf("  ✗ Model weights not found for %s (%s)\n", rec.ModelName, rec.HFFile)
		res.Issues = append(res.Issues, fmt.Sprintf("Model weights missing: %s", rec.HFFile))
		if opts.DownloadModel {
			fmt.Printf("  Downloading weights from Hugging Face: %s/%s...\n", rec.HFRepo, rec.HFFile)
			url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", rec.HFRepo, rec.HFFile)
			if err := downloadFile(url, targetModelPath); err != nil {
				fmt.Printf("  Failed downloading model: %v\n", err)
			} else {
				res.ModelInstalled = true
				fmt.Printf("  ✓ Successfully downloaded to: %s\n", targetModelPath)
			}
		}
	}

	// 4. Engine Service & Health
	fmt.Println()
	fmt.Println("[4/4] Checking engine connectivity...")
	active, engineErr := checkEngineHealth("http://127.0.0.1:8080")
	res.ServiceActive = active
	if active {
		fmt.Println("  ✓ llama-server is currently active and healthy on http://127.0.0.1:8080")
	} else {
		fmt.Println("  ⚠️  llama-server is not currently running on http://127.0.0.1:8080")
		if engineErr != nil {
			fmt.Printf("     (%v)\n", engineErr)
		}
		res.Issues = append(res.Issues, "Engine not running on http://127.0.0.1:8080")
	}

	// Summary
	fmt.Println()
	fmt.Println("--------------------------------------------------")
	if len(res.Issues) == 0 {
		fmt.Println("✓ All environment requirements satisfied! Ready to run lokol.")
	} else {
		fmt.Println("Setup Notice:")
		for _, issue := range res.Issues {
			fmt.Printf("  - %s\n", issue)
		}
	}
	fmt.Println("--------------------------------------------------")
	return res, nil
}

func findLlamaBinary() (string, bool) {
	// 1. Check PATH
	if p, err := exec.LookPath("llama"); err == nil {
		return p, true
	}
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p, true
	}

	// 2. Check ~/.local/bin/llama
	home := os.Getenv("HOME")
	candidates := []string{
		filepath.Join(home, ".local", "bin", "llama"),
		filepath.Join(home, ".local", "bin", "llama-server"),
		"/usr/local/bin/llama",
		"/usr/local/bin/llama-server",
	}

	for _, cand := range candidates {
		if fileExists(cand) {
			return cand, true
		}
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Size() > 0
}

func checkEngineHealth(url string) (bool, error) {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func downloadFile(url, destPath string) error {
	out, err := os.Create(destPath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "lokol-installer/"+runtime.GOOS)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	_ = out.Close()
	return os.Rename(destPath+".tmp", destPath)
}
