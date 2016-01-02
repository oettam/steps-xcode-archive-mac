require 'optparse'
require 'plist'
require 'json'

# -----------------------
# --- functions
# -----------------------

def fail_with_message(message)
  puts "\e[31m#{message}\e[0m"
  exit(1)
end

# -----------------------
# --- main
# -----------------------

puts

# Input validation
options = {
  export_options_path: nil,
  archive_path: nil,
  export_method: nil
}

parser = OptionParser.new do|opts|
  opts.banner = 'Usage: step.rb [options]'
  opts.on('-o', '--export_options_path path', 'Export options path') { |o| options[:export_options_path] = o unless o.to_s == '' }
  opts.on('-a', '--archive_path path', 'Archive path') { |a| options[:archive_path] = a unless a.to_s == '' }
  opts.on('-e', '--export_method method', 'Export method') { |a| options[:export_method] = a unless a.to_s == '' }
  opts.on('-h', '--help', 'Displays Help') do
    puts opts
    exit
  end
end
parser.parse!

fail_with_message('export_options_path not specified') unless options[:export_options_path]
puts "(i) export_options_path: #{options[:export_options_path]}"

fail_with_message('archive_path not specified') unless options[:archive_path]
puts "(i) archive_path: #{options[:archive_path]}"

method = options[:export_method]

puts
puts '==> Create export options'

export_options = {}
# export_options[:teamID] = team_id unless team_id.nil?
export_options[:method] = method unless method.nil?

puts
puts " (i) export_options: #{export_options}"
plist_content = Plist::Emit.dump(export_options)
puts " (i) plist_content: #{plist_content}"
puts " (i) saving into file: #{options[:export_options_path]}"
File.write(options[:export_options_path], plist_content)
