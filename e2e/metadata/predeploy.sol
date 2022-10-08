pragma solidity ^0.8.7;

// A test contract for predeploy testing
contract HRM {
    struct Group {
        string name;
        uint128 number;
        bool flag;
    }

    struct Human {
        address addr;
        string name;
        int128 number;
    }

    int256 private id;
    string private name;

    Group[] private groups;
    mapping(uint256 => mapping(uint256 => Human)) people;

    struct ArgGroup {
        string name;
        uint128 number;
        bool flag;
        Human[] people;
    }

    constructor(
        int256 _id,
        string memory _name,
        ArgGroup[] memory _groups
    ) {
        id = _id;
        name = _name;

        for (uint256 idx = 0; idx < _groups.length; idx++) {
            groups.push(
                Group({
                    name: _groups[idx].name,
                    number: _groups[idx].number,
                    flag: _groups[idx].flag
                })
            );

            for (uint256 jdx = 0; jdx < _groups[idx].people.length; jdx++) {
                people[idx][jdx] = Human({
                    addr: _groups[idx].people[jdx].addr,
                    name: _groups[idx].people[jdx].name,
                    number: _groups[idx].people[jdx].number
                });
            }
        }
    }

    function getID() public view returns (int256) {
        return id;
    }

    function getName() public view returns (string memory) {
        return name;
    }

    function getGroupName(uint256 index) public view returns (string memory) {
        return groups[index].name;
    }

    function getGroupNumber(uint256 index) public view returns (uint128) {
        return groups[index].number;
    }

    function getGroupFlag(uint256 index) public view returns (bool) {
        return groups[index].flag;
    }

    function getHumanAddr(uint256 groupIndex, uint256 humanIndex)
        public
        view
        returns (address)
    {
        return people[groupIndex][humanIndex].addr;
    }

    function getHumanName(uint256 groupIndex, uint256 humanIndex)
        public
        view
        returns (string memory)
    {
        return people[groupIndex][humanIndex].name;
    }

    function getHumanNumber(uint256 groupIndex, uint256 humanIndex)
        public
        view
        returns (int128)
    {
        return people[groupIndex][humanIndex].number;
    }
}
