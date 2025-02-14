package hashcat_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/pkg/hashcat"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/stretchr/testify/assert"
)

func TestAppendCharsets(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    cmdArgs := []string{}
    charsets := []string{"charset1", "charset2",
                         "charset3", "charset4"}
    // Format and append the charsets as command line args
    hashcat.AppendCharsets(&cmdArgs, charsets)
    // Ensure the charsets were properly parsed
    assert.Equal("-1", cmdArgs[0])
    assert.Equal("charset1", cmdArgs[1])
    assert.Equal("-2", cmdArgs[2])
    assert.Equal("charset2", cmdArgs[3])
    assert.Equal("-3", cmdArgs[4])
    assert.Equal("charset3", cmdArgs[5])
    assert.Equal("-4", cmdArgs[6])
    assert.Equal("charset4", cmdArgs[7])
}


func TestParseHashcatOutput(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    var hashcatOut = []byte(`
hashcat (v6.2.6) starting$
$
OpenCL API (OpenCL 3.0 PoCL 6.0+debian  Linux, None+Asserts, RELOC, LLVM 18.1.8, SLEEF, DISTRO, POCL_DEBUG) - Platform #1 [The pocl project]$
============================================================================================================================================$
* Device #1: cpu-haswell-AMD A12-9700P RADEON R7, 10 COMPUTE CORES 4C+6G, 2644/5353 MB (1024 MB allocatable), 4MCU$
$
Minimum password length supported by kernel: 0$
Maximum password length supported by kernel: 31$
$
Counting lines in hash. Please be patient...Counted lines in hashParsed Hashes: 1/1 (100.00%)Parsed Hashes: 1/1 (100.00%)Sorting hashes. Please be patient...Sorted hashesRemoving duplicate hashes. Please be patient...Removed duplicate hashesSorting salts. Please be patient...Sorted saltsComparing hashes with potfile entries. Please be patient...Compared hashes with potfile entriesGenerating bitmap tables...Generated bitmap tablesHashes: 1 digests; 1 unique digests, 1 unique salts$
Bitmaps: 16 bits, 65536 entries, 0x0000ffff mask, 262144 bytes, 5/13 rotates$
Rules: 1$
$
Optimizers applied:$
* Optimized-Kernel$
* Zero-Byte$
* Precompute-Init$
* Early-Skip$
* Not-Salted$
* Not-Iterated$
* Single-Hash$
* Single-Salt$
* Raw-Hash$
* Uses-64-Bit$
$
Watchdog: Temperature abort trigger set to 90c$
$
Initializing device kernels and memory. Please be patient...Initializing backend runtime for device #1. Please be patient...Initialized backend runtime for device #1Host memory required for this attack: 1 MB$
$
Initialized device kernels and memoryStarting self-test. Please be patient...Finished self-testDictionary cache hit:$
* Filename..: /usr/share/wordlists/rockyou.txt$
* Passwords.: 14344385$
* Bytes.....: 139921507$
* Keyspace..: 14344385$
$
Starting autotune. Please be patient...Finished autotune^M                                                          ^M[s]tatus [p]ause [b]ypass [c]heckpoint [f]inish [q]uit => ^M                                                          ^M$
Session..........: hashcat$
Status...........: Cracked$
Hash.Mode........: 1700 (SHA2-512)$
Hash.Target......: ab6a34a451b061203fb9e7cbeae89f8b94c4d42ee38a88fbd6f...5d9a02$
Time.Started.....: Wed Feb 12 23:01:45 2025 (0 secs)$
Time.Estimated...: Wed Feb 12 23:01:45 2025 (0 secs)$
Kernel.Feature...: Optimized Kernel$
Guess.Base.......: File (/usr/share/wordlists/rockyou.txt)$
Guess.Queue......: 1/1 (100.00%)$
Speed.#1.........:   487.0 kH/s (1.96ms) @ Accel:512 Loops:1 Thr:1 Vec:4$
Recovered........: 1/1 (100.00%) Digests (total), 1/1 (100.00%) Digests (new)$
Progress.........: 2048/14344385 (0.01%)$
Rejected.........: 0/2048 (0.00%)$
Restore.Point....: 0/14344385 (0.00%)$
Restore.Sub.#1...: Salt:0 Amplifier:0-1 Iteration:0-1$
Candidate.Engine.: Device Generator$
Candidates.#1....: 123456 -> lovers1$
Hardware.Mon.#1..: Temp: 67c Util: 25%$
$
Started: Wed Feb 12 23:01:43 2025$
Stopped: Wed Feb 12 23:01:47 2025$
`)
    logArgs := hashcat.ParseHashcatOutput(hashcatOut, []byte("=>"))
    assert.Greater(len(logArgs), 0)

    region := "test-region"
    awsAccessKey := "blah"
    awsSecretKey := "blah"
    awsCreds := credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")

    // Load default config and override with custom credentials and region
    awsConfig, err := config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(region),
        config.WithCredentialsProvider(awsCreds),
    )
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager("local", "", awsConfig, true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Log the hashcat output with kloudlogs
    kloudlogs.LogMessage(logMan, "info", "TestParseHashcatOutput test message", logArgs...)
    // Get the log message from memory and parse it as a map
    logMap, err := kloudlogs.LogToMap(logMan.GetLog())
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the key-values were properly parsed
    assert.Equal(logMap["Session"], "hashcat")
    assert.Equal(logMap["Status"], "Cracked")
    assert.Equal(logMap["Hash.Mode"], "1700 (SHA2-512)")
    assert.Equal(logMap["Hash.Target"], "ab6a34a451b061203fb9e7cbeae89f8b94c4d42ee38a88fbd6f...5d9a02")
    assert.Equal(logMap["Time.Started"], "Wed Feb 12 23:01:45 2025 (0 secs)")
    assert.Equal(logMap["Time.Estimated"], "Wed Feb 12 23:01:45 2025 (0 secs)")
    assert.Equal(logMap["Kernel.Feature"], "Optimized Kernel")
    assert.Equal(logMap["Guess.Base"], "File (/usr/share/wordlists/rockyou.txt)")
    assert.Equal(logMap["Guess.Queue"], "1/1 (100.00%)")
    assert.Equal(logMap["Speed.#1"], "487.0 kH/s (1.96ms) @ Accel:512 Loops:1 Thr:1 Vec:4")
    assert.Equal(logMap["Recovered"], "1/1 (100.00%) Digests (total), 1/1 (100.00%) Digests (new)")
    assert.Equal(logMap["Progress"], "2048/14344385 (0.01%)")
    assert.Equal(logMap["Rejected"], "0/2048 (0.00%)")
    assert.Equal(logMap["Restore.Point"], "0/14344385 (0.00%)")
    assert.Equal(logMap["Restore.Sub.#1"], "Salt:0 Amplifier:0-1 Iteration:0-1")
    assert.Equal(logMap["Candidate.Engine"], "Device Generator")
    assert.Equal(logMap["Candidates.#1"], "123456 -> lovers1")
    assert.Equal(logMap["Hardware.Mon.#1"], "Temp: 67c Util: 25%")
}
