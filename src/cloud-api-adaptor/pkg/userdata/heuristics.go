package userdata

import (
	"context"
	"fmt"
	"os"
	"time"

	retry "github.com/avast/retry-go/v4"
	. "github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/paths"
	dmidecode "github.com/fenglyu/go-dmidecode"
	cpuid "github.com/klauspost/cpuid/v2"
)

func isAzureVM() bool {
	return cpuid.CPU.HypervisorVendorID == cpuid.MSVM
}

func isAWSVM(ctx context.Context) bool {
	t, err := dmidecode.NewDMITable()
	if err != nil {
		return false
	}

	provider := t.Query(dmidecode.KeywordSystemManufacturer)

	return provider == "Amazon EC2"
}

func isGCPVM(ctx context.Context) bool {
	if cpuid.CPU.HypervisorVendorID != cpuid.KVM {
		return false
	}
	_, err := imdsGet(ctx, GcpImdsUrl, false, []kvPair{{"Metadata-Flavor", "Google"}})
	return err == nil
}

func hasUserDataFile(ctx context.Context) bool {
	path := UserDataPath
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	err := retry.Do(
		func() error {
			logger.Printf("Checking if file is present at path: %s\n", path)
			_, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("failed to stat file %s: %w", path, err)
			}
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.MaxDelay(5*time.Second),
	)

	if err != nil {
		logger.Println(err)
		return false
	}
	logger.Printf("User data file is present at path %s\n", path)
	return true
}

func isAlibabaCloudVM() bool {
	t, err := dmidecode.NewDMITable()
	if err != nil {
		return false
	}

	provider := t.Query(dmidecode.KeywordSystemManufacturer)

	return provider == "Alibaba Cloud"
}
