import matplotlib.pyplot as plt
import re

def parse_data(filename):
    processed_ips = []
    estimates = []
    true_values = []
    errors = []
    times = []
    
    with open(filename, 'r') as file:
        for line in file:
            # Use regex to extract all numerical values
            numbers = re.findall(r'\d+\.?\d*', line)
            
            if len(numbers) >= 5:
                processed_ips.append(int(numbers[0]))
                estimates.append(int(numbers[1]))
                true_values.append(int(numbers[2]))
                errors.append(float(numbers[3]))
                times.append(float(numbers[4]))
    
    return processed_ips, estimates, true_values, errors, times

def plot_data(processed_ips, estimates, true_values, errors, times):
    fig, ((ax1, ax2), (ax3, ax4)) = plt.subplots(2, 2, figsize=(15, 10))
    
    # Plot 1: Estimates vs True values
    ax1.plot(processed_ips, estimates, 'b-', label='Estimate', alpha=0.7)
    ax1.plot(processed_ips, true_values, 'r-', label='True', alpha=0.7)
    ax1.set_xlabel('Processed IPs')
    ax1.set_ylabel('Count')
    ax1.set_title('Estimate vs True Values')
    ax1.legend()
    ax1.grid(True, alpha=0.3)
    
    # Plot 2: Error percentage over time
    ax2.plot(processed_ips, errors, 'g-')
    ax2.set_xlabel('Processed IPs')
    ax2.set_ylabel('Error (%)')
    ax2.set_title('Error Percentage vs Processed IPs')
    ax2.grid(True, alpha=0.3)
    
    # Plot 3: Processing time
    ax3.plot(processed_ips, times, 'purple')
    ax3.set_xlabel('Processed IPs')
    ax3.set_ylabel('Time (seconds)')
    ax3.set_title('Processing Time vs Processed IPs')
    ax3.grid(True, alpha=0.3)
    
    # Plot 4: Difference between Estimate and True values
    differences = [est - true for est, true in zip(estimates, true_values)]
    ax4.plot(processed_ips, differences, 'orange')
    ax4.set_xlabel('Processed IPs')
    ax4.set_ylabel('Difference (Estimate - True)')
    ax4.set_title('Difference between Estimate and True Values')
    ax4.grid(True, alpha=0.3)
    
    plt.tight_layout()
    plt.show()

def plot_combined_comparison(processed_ips, estimates, true_values, errors):
    """Create a combined plot for better comparison"""
    plt.figure(figsize=(12, 8))
    
    # Plot estimates and true values
    plt.subplot(2, 1, 1)
    plt.plot(processed_ips, estimates, 'b-', label='Estimate', alpha=0.8, linewidth=2)
    plt.plot(processed_ips, true_values, 'r-', label='True', alpha=0.8, linewidth=2)
    plt.xlabel('Processed IPs')
    plt.ylabel('Count')
    plt.title('Cardinality Estimation: Estimate vs True Values')
    plt.legend()
    plt.grid(True, alpha=0.3)
    
    # Plot error percentage
    plt.subplot(2, 1, 2)
    plt.plot(processed_ips, errors, 'g-', linewidth=2)
    plt.xlabel('Processed IPs')
    plt.ylabel('Error (%)')
    plt.title('Estimation Error Over Time')
    plt.grid(True, alpha=0.3)
    
    plt.tight_layout()
    plt.show()

# Main execution
if __name__ == "__main__":
    filename = 'benchmarking/benchmarks.txt'
    
    try:
        # Parse the data
        processed_ips, estimates, true_values, errors, times = parse_data(filename)
        
        # Print some statistics
        print(f"Total data points: {len(processed_ips)}")
        print(f"Final processed IPs: {processed_ips[-1]:,}")
        print(f"Final error: {errors[-1]:.2f}%")
        print(f"Max error: {max(errors):.2f}%")
        print(f"Average error: {sum(errors)/len(errors):.2f}%")
        
        # Plot the data
        plot_data(processed_ips, estimates, true_values, errors, times)
        
        # Additional combined plot
        plot_combined_comparison(processed_ips, estimates, true_values, errors)
        
    except FileNotFoundError:
        print(f"File '{filename}' not found. Please check the file path.")
    except Exception as e:
        print(f"An error occurred: {e}")