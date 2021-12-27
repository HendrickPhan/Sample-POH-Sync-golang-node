# Node

1. Receive Tick from validator
2. Validate POH of tick 
3. send result to validator
4. Receive confirm block from validator
5. Send Checked block to validator


# How to run:
1. Build
    ./buildNodes.sh
2. Config which validator node will connection in config file of each node in build folder
3. Run bin file in each node folder

# Note:  
Must run in this sequence to make node work correctly:
1. Start Validator1
2. Start Node1
3. Start validator2
3. Start Node2