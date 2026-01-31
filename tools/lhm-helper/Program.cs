using System.Text.Json;
using LibreHardwareMonitor.Hardware;

namespace LhmHelper;

/// <summary>
/// Helper program to collect CPU temperature using LibreHardwareMonitor.
/// Outputs JSON to stdout for consumption by ResourceAgent.
/// Requires administrator privileges and PawnIO driver.
/// </summary>
class Program
{
    static int Main(string[] args)
    {
        try
        {
            var result = CollectCpuTemperatures();
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

    static TemperatureResult CollectCpuTemperatures()
    {
        var sensors = new List<SensorData>();

        var computer = new Computer
        {
            IsCpuEnabled = true
        };

        try
        {
            computer.Open();

            foreach (var hardware in computer.Hardware)
            {
                if (hardware.HardwareType != HardwareType.Cpu)
                    continue;

                hardware.Update();

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
        }
        finally
        {
            computer.Close();
        }

        return new TemperatureResult { Sensors = sensors };
    }
}

class TemperatureResult
{
    public List<SensorData> Sensors { get; set; } = new();
}

class SensorData
{
    public string Name { get; set; } = string.Empty;
    public double Temperature { get; set; }
    public double High { get; set; }
    public double Critical { get; set; }
}
