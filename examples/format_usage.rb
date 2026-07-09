# frozen_string_literal: true

# Kernel#format / Kernel#sprintf / String#% are core methods, so no require is
# needed; this "format" library is the pure-Go engine backing all three in rbgo.

# Width, precision, zero-pad, and a hex conversion in one template.
puts format("%05.2f -> 0x%x", 3.14159, 255) # => 03.14 -> 0xff

# sprintf is an alias of format.
puts sprintf("%d apples, %d oranges", 3, 5) # => 3 apples, 5 oranges

# String#% with flags: left-justify, and forced sign.
puts("%-10s|" % "left")   # => left      |
puts("%+d %+d" % [7, -7]) # => +7 -7

# Named references pull from a hash operand.
puts("%<name>s is %<age>d" % { name: "Ada", age: 36 }) # => Ada is 36

# Binary, octal, and hex integer conversions.
puts("%b %o %x %X" % [10, 10, 255, 255]) # => 1010 12 ff FF

# Arbitrary-precision integers format at full width.
puts("%d" % (10**30)) # => 1000000000000000000000000000000

# Absolute argument references reorder operands.
puts("%3$s %2$s %1$s" % %w[a b c]) # => c b a

# A literal percent needs %%.
puts format("Progress: 100%% complete") # => Progress: 100% complete

# MRI-matching errors surface as Ruby exceptions.
begin
  sprintf("%d")
rescue ArgumentError => e
  puts "ArgumentError: #{e.message}" # => ArgumentError: too few arguments
end
