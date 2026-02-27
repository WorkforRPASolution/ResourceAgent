using System.Text;
using System.Text.Json;
using LibreHardwareMonitor.Hardware;

namespace LhmHelper;

/// <summary>
/// Helper program to collect hardware metrics using LibreHardwareMonitor.
/// Outputs JSON to stdout for consumption by ResourceAgent.
/// Requires administrator privileges and PawnIO driver.
///
/// Modes:
///   (default)    One-shot: collect once, print JSON, exit.
///   --daemon     Long-running: read stdin lines, respond with JSON per line.
/// </summary>
class Program
{
    static int Main(string[] args)
    {
        if (args.Contains("--daemon", StringComparer.OrdinalIgnoreCase))
        {
            return RunDaemon();
        }

        // One-shot mode (backward compatible)
        try
        {
            var result = CollectHardwareMetrics();
            Console.WriteLine(JsonSerializer.Serialize(result));
            return 0;
        }
        catch (Exception ex)
        {
            var error = new { error = ex.Message };
            Console.WriteLine(JsonSerializer.Serialize(error));
            return 1;
        }
    }

    /// <summary>
    /// Daemon mode: computer.Open() once, then respond to stdin requests via stdout.
    /// Exits when stdin is closed (parent process died or graceful shutdown).
    /// Logs to stderr (stdout is the data channel).
    /// </summary>
    static int RunDaemon()
    {
        LogStderr("LhmHelper daemon starting");

        var computer = new Computer
        {
            IsCpuEnabled = true,
            IsMotherboardEnabled = true,
            IsGpuEnabled = true,
            IsStorageEnabled = true
        };

        try
        {
            computer.Open();
            LogStderr("computer.Open() succeeded");
        }
        catch (Exception ex)
        {
            LogStderr($"computer.Open() failed: {ex.Message}");
            Console.WriteLine(JsonSerializer.Serialize(new { error = $"computer.Open() failed: {ex.Message}" }));
            Console.Out.Flush();
            return 1;
        }

        // Track sensor counts to log only when they change
        int prevSensorCount = -1;
        int prevFanCount = -1;
        int prevGpuCount = -1;
        int prevStorageCount = -1;
        int prevVoltageCount = -1;
        int prevMbTempCount = -1;

        try
        {
            string? line;
            while ((line = Console.ReadLine()) != null)
            {
                try
                {
                    var result = CollectFromOpenComputer(computer);
                    Console.WriteLine(JsonSerializer.Serialize(result));
                    Console.Out.Flush();

                    LogSensorChanges(result,
                        ref prevSensorCount, ref prevFanCount, ref prevGpuCount,
                        ref prevStorageCount, ref prevVoltageCount, ref prevMbTempCount);
                }
                catch (Exception ex)
                {
                    LogStderr($"Collection error: {ex.Message}");
                    Console.WriteLine(JsonSerializer.Serialize(new { error = ex.Message }));
                    Console.Out.Flush();
                }
            }

            LogStderr("stdin closed, shutting down");
        }
        finally
        {
            try
            {
                computer.Close();
                LogStderr("computer.Close() succeeded");
            }
            catch (Exception ex)
            {
                LogStderr($"computer.Close() error: {ex.Message}");
            }
        }

        return 0;
    }

    /// <summary>
    /// One-shot collection: Open, collect, Close.
    /// </summary>
    static HardwareResult CollectHardwareMetrics()
    {
        var computer = new Computer
        {
            IsCpuEnabled = true,
            IsMotherboardEnabled = true,
            IsGpuEnabled = true,
            IsStorageEnabled = true
        };

        try
        {
            computer.Open();
            return CollectFromOpenComputer(computer);
        }
        finally
        {
            computer.Close();
        }
    }

    /// <summary>
    /// Collect all hardware metrics from an already-open Computer.
    /// Used by both one-shot and daemon modes.
    /// </summary>
    static HardwareResult CollectFromOpenComputer(Computer computer)
    {
        var sensors = new List<SensorData>();
        var fans = new List<FanData>();
        var gpus = new List<GpuData>();
        var storages = new List<StorageData>();
        var voltages = new List<VoltageData>();
        var motherboardTemps = new List<MotherboardTempData>();

        foreach (var hardware in computer.Hardware)
        {
            hardware.Update();

            // Collect CPU temperatures
            if (hardware.HardwareType == HardwareType.Cpu)
            {
                foreach (var sensor in hardware.Sensors)
                {
                    if (sensor.SensorType != SensorType.Temperature)
                        continue;

                    if (!sensor.Value.HasValue)
                        continue;

                    var temperature = sensor.Value.Value;

                    // Skip invalid readings
                    if (temperature <= 0 || temperature > 200)
                        continue;

                    sensors.Add(new SensorData
                    {
                        Name = $"{hardware.Name} - {sensor.Name}",
                        Temperature = Math.Round(temperature, 1),
                        High = sensor.Parameters.FirstOrDefault(p => p.Name == "TjMax")?.Value ?? 100.0,
                        Critical = 105.0 // Conservative critical threshold
                    });
                }

                // Also check sub-hardware (for multi-die CPUs)
                foreach (var subHardware in hardware.SubHardware)
                {
                    subHardware.Update();

                    foreach (var sensor in subHardware.Sensors)
                    {
                        if (sensor.SensorType != SensorType.Temperature)
                            continue;

                        if (!sensor.Value.HasValue)
                            continue;

                        var temperature = sensor.Value.Value;

                        if (temperature <= 0 || temperature > 200)
                            continue;

                        sensors.Add(new SensorData
                        {
                            Name = $"{subHardware.Name} - {sensor.Name}",
                            Temperature = Math.Round(temperature, 1),
                            High = 100.0,
                            Critical = 105.0
                        });
                    }
                }
            }

            // Collect Fan speeds, Voltage, and Temperatures from Motherboard
            if (hardware.HardwareType == HardwareType.Motherboard)
            {
                // Check sub-hardware (SuperIO chips contain fan sensors)
                foreach (var subHardware in hardware.SubHardware)
                {
                    subHardware.Update();

                    foreach (var sensor in subHardware.Sensors)
                    {
                        if (!sensor.Value.HasValue)
                            continue;

                        var value = sensor.Value.Value;

                        // Collect Fan speeds
                        if (sensor.SensorType == SensorType.Fan)
                        {
                            if (value < 0)
                                continue;

                            fans.Add(new FanData
                            {
                                Name = sensor.Name,
                                RPM = Math.Round(value, 0)
                            });
                        }
                        // Collect Voltage
                        else if (sensor.SensorType == SensorType.Voltage)
                        {
                            if (value <= 0)
                                continue;

                            voltages.Add(new VoltageData
                            {
                                Name = sensor.Name,
                                Voltage = Math.Round(value, 3)
                            });
                        }
                        // Collect Motherboard Temperature
                        else if (sensor.SensorType == SensorType.Temperature)
                        {
                            if (value <= 0 || value > 200)
                                continue;

                            motherboardTemps.Add(new MotherboardTempData
                            {
                                Name = $"{subHardware.Name} - {sensor.Name}",
                                Temperature = Math.Round(value, 1)
                            });
                        }
                    }
                }

                // Also check direct sensors on motherboard hardware
                foreach (var sensor in hardware.Sensors)
                {
                    if (!sensor.Value.HasValue)
                        continue;

                    var value = sensor.Value.Value;

                    if (sensor.SensorType == SensorType.Fan)
                    {
                        if (value < 0)
                            continue;

                        fans.Add(new FanData
                        {
                            Name = sensor.Name,
                            RPM = Math.Round(value, 0)
                        });
                    }
                    else if (sensor.SensorType == SensorType.Voltage)
                    {
                        if (value <= 0)
                            continue;

                        voltages.Add(new VoltageData
                        {
                            Name = sensor.Name,
                            Voltage = Math.Round(value, 3)
                        });
                    }
                    else if (sensor.SensorType == SensorType.Temperature)
                    {
                        if (value <= 0 || value > 200)
                            continue;

                        motherboardTemps.Add(new MotherboardTempData
                        {
                            Name = $"{hardware.Name} - {sensor.Name}",
                            Temperature = Math.Round(value, 1)
                        });
                    }
                }
            }

            // Collect GPU metrics
            if (hardware.HardwareType == HardwareType.GpuNvidia ||
                hardware.HardwareType == HardwareType.GpuAmd ||
                hardware.HardwareType == HardwareType.GpuIntel)
            {
                var gpu = new GpuData { Name = hardware.Name };

                foreach (var sensor in hardware.Sensors)
                {
                    if (!sensor.Value.HasValue)
                        continue;

                    var value = sensor.Value.Value;

                    switch (sensor.SensorType)
                    {
                        case SensorType.Temperature:
                            if (sensor.Name.Contains("Core") || sensor.Name.Contains("GPU"))
                                gpu.Temperature = Math.Round(value, 1);
                            break;
                        case SensorType.Load:
                            if (sensor.Name.Contains("Core") || sensor.Name == "GPU Core")
                                gpu.CoreLoad = Math.Round(value, 1);
                            else if (sensor.Name.Contains("Memory"))
                                gpu.MemoryLoad = Math.Round(value, 1);
                            break;
                        case SensorType.Fan:
                            if (!gpu.FanSpeed.HasValue)
                                gpu.FanSpeed = Math.Round(value, 0);
                            break;
                        case SensorType.Power:
                            if (sensor.Name.Contains("Package") || sensor.Name.Contains("GPU"))
                                gpu.Power = Math.Round(value, 1);
                            break;
                        case SensorType.Clock:
                            if (sensor.Name.Contains("Core") || sensor.Name == "GPU Core")
                                gpu.CoreClock = Math.Round(value, 0);
                            else if (sensor.Name.Contains("Memory"))
                                gpu.MemoryClock = Math.Round(value, 0);
                            break;
                    }
                }

                gpus.Add(gpu);
            }

            // Collect Storage S.M.A.R.T data
            if (hardware.HardwareType == HardwareType.Storage)
            {
                var storage = new StorageData { Name = hardware.Name };

                // Determine storage type from name
                var nameLower = hardware.Name.ToLower();
                if (nameLower.Contains("nvme"))
                    storage.Type = "NVMe";
                else if (nameLower.Contains("ssd"))
                    storage.Type = "SSD";
                else
                    storage.Type = "HDD";

                foreach (var sensor in hardware.Sensors)
                {
                    if (!sensor.Value.HasValue)
                        continue;

                    var value = sensor.Value.Value;
                    var sensorNameLower = sensor.Name.ToLower();

                    switch (sensor.SensorType)
                    {
                        case SensorType.Temperature:
                            storage.Temperature = Math.Round(value, 1);
                            break;
                        case SensorType.Level:
                            if (sensorNameLower.Contains("remaining") || sensorNameLower.Contains("life"))
                                storage.RemainingLife = Math.Round(value, 1);
                            break;
                        case SensorType.Data:
                            if (sensorNameLower.Contains("written"))
                                storage.TotalBytesWritten = (long)(value * 1024 * 1024 * 1024); // Convert GB to bytes
                            break;
                    }
                }

                // Try to get additional S.M.A.R.T attributes from raw data
                // These are often available as sensor data with specific names
                foreach (var sensor in hardware.Sensors)
                {
                    if (!sensor.Value.HasValue)
                        continue;

                    var sensorNameLower = sensor.Name.ToLower();
                    var value = (long)sensor.Value.Value;

                    if (sensorNameLower.Contains("media error") || sensorNameLower.Contains("media_error"))
                        storage.MediaErrors = value;
                    else if (sensorNameLower.Contains("power cycle") || sensorNameLower.Contains("power-on"))
                        storage.PowerCycles = value;
                    else if (sensorNameLower.Contains("unsafe shutdown"))
                        storage.UnsafeShutdowns = value;
                    else if (sensorNameLower.Contains("power on hours") || sensorNameLower.Contains("power-on hours"))
                        storage.PowerOnHours = value;
                }

                storages.Add(storage);
            }
        }

        return new HardwareResult
        {
            Sensors = sensors,
            Fans = fans,
            Gpus = gpus,
            Storages = storages,
            Voltages = voltages,
            MotherboardTemps = motherboardTemps
        };
    }

    static void LogStderr(string message)
    {
        Console.Error.WriteLine($"[{DateTime.Now:yyyy-MM-dd HH:mm:ss.fff}] {message}");
        Console.Error.Flush();
    }

    static void LogSensorChanges(HardwareResult result,
        ref int prevSensors, ref int prevFans, ref int prevGpus,
        ref int prevStorages, ref int prevVoltages, ref int prevMbTemps)
    {
        bool changed = false;
        var sb = new StringBuilder("Sensor counts changed:");

        if (result.Sensors.Count != prevSensors)
        {
            sb.Append($" temp={result.Sensors.Count}");
            prevSensors = result.Sensors.Count;
            changed = true;
        }
        if (result.Fans.Count != prevFans)
        {
            sb.Append($" fan={result.Fans.Count}");
            prevFans = result.Fans.Count;
            changed = true;
        }
        if (result.Gpus.Count != prevGpus)
        {
            sb.Append($" gpu={result.Gpus.Count}");
            prevGpus = result.Gpus.Count;
            changed = true;
        }
        if (result.Storages.Count != prevStorages)
        {
            sb.Append($" storage={result.Storages.Count}");
            prevStorages = result.Storages.Count;
            changed = true;
        }
        if (result.Voltages.Count != prevVoltages)
        {
            sb.Append($" voltage={result.Voltages.Count}");
            prevVoltages = result.Voltages.Count;
            changed = true;
        }
        if (result.MotherboardTemps.Count != prevMbTemps)
        {
            sb.Append($" mb_temp={result.MotherboardTemps.Count}");
            prevMbTemps = result.MotherboardTemps.Count;
            changed = true;
        }

        if (changed)
            LogStderr(sb.ToString());
    }
}

class HardwareResult
{
    public List<SensorData> Sensors { get; set; } = new();
    public List<FanData> Fans { get; set; } = new();
    public List<GpuData> Gpus { get; set; } = new();
    public List<StorageData> Storages { get; set; } = new();
    public List<VoltageData> Voltages { get; set; } = new();
    public List<MotherboardTempData> MotherboardTemps { get; set; } = new();
}

class SensorData
{
    public string Name { get; set; } = string.Empty;
    public double Temperature { get; set; }
    public double High { get; set; }
    public double Critical { get; set; }
}

class FanData
{
    public string Name { get; set; } = string.Empty;
    public double RPM { get; set; }
}

class GpuData
{
    public string Name { get; set; } = string.Empty;
    public double? Temperature { get; set; }
    public double? CoreLoad { get; set; }
    public double? MemoryLoad { get; set; }
    public double? FanSpeed { get; set; }
    public double? Power { get; set; }
    public double? CoreClock { get; set; }
    public double? MemoryClock { get; set; }
}

class StorageData
{
    public string Name { get; set; } = string.Empty;
    public string Type { get; set; } = string.Empty; // NVMe, SSD, HDD
    public double? Temperature { get; set; }
    public double? RemainingLife { get; set; } // Percentage
    public long? MediaErrors { get; set; }
    public long? PowerCycles { get; set; }
    public long? UnsafeShutdowns { get; set; }
    public long? PowerOnHours { get; set; }
    public long? TotalBytesWritten { get; set; }
}

class VoltageData
{
    public string Name { get; set; } = string.Empty;
    public double Voltage { get; set; }
}

class MotherboardTempData
{
    public string Name { get; set; } = string.Empty;
    public double Temperature { get; set; }
}
