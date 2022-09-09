// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@chainlink/contracts/src/v0.8/interfaces/AggregatorValidatorInterface.sol";
import "@chainlink/contracts/src/v0.8/interfaces/TypeAndVersionInterface.sol";
import "@chainlink/contracts/src/v0.8/interfaces/AccessControllerInterface.sol";
import "@chainlink/contracts/src/v0.8/interfaces/AggregatorV3Interface.sol";
import "@chainlink/contracts/src/v0.8/SimpleWriteAccessController.sol";
import "@chainlink/contracts/src/v0.8/dev/vendor/openzeppelin-solidity/v4.3.1/contracts/utils/Address.sol";
import "../../../../vendor/starkware-libs/starkgate-contracts-solidity-v0.8/src/starkware/starknet/solidity/IStarknetMessaging.sol";

/// @title StarkNetValidator - makes cross chain calls to update the Sequencer Uptime Feed on L2
contract StarkNetValidator is TypeAndVersionInterface, AggregatorValidatorInterface, SimpleWriteAccessController {
  int256 private constant ANSWER_SEQ_OFFLINE = 1;
  // The selector is the starknet_keccak hash of the function name 'update_status'.
  // Notice: hardcoded b/c starknet_keccak is not available in this environment.
  uint256 constant STARK_SELECTOR_UPDATE_STATUS =
    1585322027166395525705364165097050997465692350398750944680096081848180365267;

  struct GasConfig {
    uint256 gasUsed;
    address l1GasPriceFeedAddr;
  }

  IStarknetMessaging public immutable STARKNET_CROSS_DOMAIN_MESSENGER;
  uint256 public immutable L2_UPTIME_FEED_ADDR;

  GasConfig private s_gasConfig;
  AccessControllerInterface private s_configAC;

  /// @notice StarkNet messaging contract address - the address is 0.
  error InvalidStarkNetMessaging();
  /// @notice StarkNet uptime feed address - the address is 0.
  error InvalidUptimeFeedAddress();

  /**
   * @param starkNetMessaging the address of the StarkNet Messaging contract address
   * @param l2UptimeFeedAddr the address of the Sequencer Uptime Feed on L2
   */
  constructor(
    address starkNetMessaging,
    address configACAddr,
    address l1GasPriceFeedAddr,
    uint256 l2UptimeFeedAddr,
    uint256 gasUsed
  ) {
    if (starkNetMessaging == address(0)) {
      revert InvalidStarkNetMessaging();
    }

    if (l2UptimeFeedAddr == 0) {
      revert InvalidUptimeFeedAddress();
    }

    STARKNET_CROSS_DOMAIN_MESSENGER = IStarknetMessaging(starkNetMessaging);
    L2_UPTIME_FEED_ADDR = l2UptimeFeedAddr;

    _setConfigAC(configACAddr);
    _setGasConfig(gasUsed, l1GasPriceFeedAddr);
  }

  /// @notice converts a bool to uint256.
  function toUInt256(bool x) internal pure returns (uint256 r) {
    assembly {
      r := x
    }
  }

  /**
   * @notice versions:
   *
   * - StarkNetValidator 0.1.0: initial release
   * @inheritdoc TypeAndVersionInterface
   */
  function typeAndVersion() external pure virtual override returns (string memory) {
    return "StarkNetValidator 0.1.0";
  }

  /**
   * @notice Returns the gas configuration for sending cross chain messages.
   */
  function getGasConfig() external view returns (GasConfig memory) {
    return s_gasConfig;
  }

  /// @return config AccessControllerInterface contract address
  function getConfigAC() external view virtual returns (address) {
    return address(s_configAC);
  }

  /**
   * @notice validate method sends an xDomain L2 tx to update Uptime Feed contract on L2.
   * @dev A message is sent using the L1CrossDomainMessenger. This method is accessed controlled.
   * @param currentAnswer new aggregator answer - value of 1 considers the sequencer offline.
   */
  function validate(
    uint256, /* previousRoundId */
    int256, /* previousAnswer */
    uint256, /* currentRoundId */
    int256 currentAnswer
  ) external override checkAccess returns (bool) {
    bool status = currentAnswer == ANSWER_SEQ_OFFLINE;
    uint256[] memory payload = new uint256[](2);

    // Fill payload with `status` and `timestamp`
    payload[0] = toUInt256(status);
    payload[1] = block.timestamp;

    uint256 fee = _approximateFee();
    // Make the StarkNet x-domain call.
    // NOTICE: we ignore the output of this call (msgHash, nonce), and we don't raise any events as the event LogMessageToL2 will be emitted from the messaging contract
    STARKNET_CROSS_DOMAIN_MESSENGER.sendMessageToL2{value: fee}(
      L2_UPTIME_FEED_ADDR,
      STARK_SELECTOR_UPDATE_STATUS,
      payload
    );
    return true;
  }
}
