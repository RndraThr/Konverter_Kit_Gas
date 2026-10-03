package bast

import "konkit/internal/textnorm"

func renderBusinessText(value string) string {
	return textnorm.BusinessUpper(value)
}

func uppercaseRecipientDocument(document RecipientDocument) RecipientDocument {
	snapshot := document.Snapshot
	snapshot.Recipient.FullName = renderBusinessText(snapshot.Recipient.FullName)
	snapshot.Recipient.SectorIdentifier = renderBusinessText(snapshot.Recipient.SectorIdentifier)
	snapshot.Recipient.Address = renderBusinessText(snapshot.Recipient.Address)
	snapshot.Recipient.Village = renderBusinessText(snapshot.Recipient.Village)
	snapshot.Recipient.District = renderBusinessText(snapshot.Recipient.District)
	snapshot.Recipient.Regency = renderBusinessText(snapshot.Recipient.Regency)
	snapshot.Equipment.MachineBrand = renderBusinessText(snapshot.Equipment.MachineBrand)
	snapshot.Equipment.MachineType = renderBusinessText(snapshot.Equipment.MachineType)
	snapshot.Equipment.MachineSerial = renderBusinessText(snapshot.Equipment.MachineSerial)
	snapshot.Equipment.HoseBrand = renderBusinessText(snapshot.Equipment.HoseBrand)
	snapshot.Equipment.HoseSpec = renderBusinessText(snapshot.Equipment.HoseSpec)
	snapshot.Equipment.HoseSerial = renderBusinessText(snapshot.Equipment.HoseSerial)
	snapshot.Equipment.ConverterBrand = renderBusinessText(snapshot.Equipment.ConverterBrand)
	snapshot.Equipment.ConverterSerial = renderBusinessText(snapshot.Equipment.ConverterSerial)
	snapshot.Components = append([]ComponentSnapshot(nil), snapshot.Components...)
	for index := range snapshot.Components {
		snapshot.Components[index].Label = renderBusinessText(snapshot.Components[index].Label)
		snapshot.Components[index].Unit = renderBusinessText(snapshot.Components[index].Unit)
	}
	snapshot.Signatures.ReceiverName = renderBusinessText(snapshot.Signatures.ReceiverName)
	snapshot.Signatures.ExecutorName = renderBusinessText(snapshot.Signatures.ExecutorName)
	snapshot.Signatures.SupervisorName = renderBusinessText(snapshot.Signatures.SupervisorName)
	document.Snapshot = snapshot
	return document
}

func uppercaseDP3Snapshot(snapshot DP3Snapshot) DP3Snapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Recipients = append([]dp3RecipientSnapshot(nil), snapshot.Recipients...)
	for index := range snapshot.Recipients {
		recipient := &snapshot.Recipients[index]
		recipient.FullName = renderBusinessText(recipient.FullName)
		recipient.Address = renderBusinessText(recipient.Address)
		recipient.Village = renderBusinessText(recipient.Village)
		recipient.District = renderBusinessText(recipient.District)
		recipient.Regency = renderBusinessText(recipient.Regency)
		recipient.MachineBrand = renderBusinessText(recipient.MachineBrand)
		recipient.MachineType = renderBusinessText(recipient.MachineType)
		recipient.MachinePower = renderBusinessText(recipient.MachinePower)
		recipient.MachineFuelType = renderBusinessText(recipient.MachineFuelType)
	}
	return snapshot
}

func uppercaseDailyRecapSnapshot(snapshot DailyRecapSnapshot) DailyRecapSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	snapshot.Recipients = append([]dailyRecapRecipientSnapshot(nil), snapshot.Recipients...)
	for index := range snapshot.Recipients {
		recipient := &snapshot.Recipients[index]
		recipient.FullName = renderBusinessText(recipient.FullName)
		recipient.FarmerCardNumber = renderBusinessText(recipient.FarmerCardNumber)
		recipient.MachineBrand = renderBusinessText(recipient.MachineBrand)
		recipient.MachineType = renderBusinessText(recipient.MachineType)
		recipient.MachineSerial = renderBusinessText(recipient.MachineSerial)
		recipient.MachinePower = renderBusinessText(recipient.MachinePower)
		recipient.MachineFuelType = renderBusinessText(recipient.MachineFuelType)
	}
	snapshot.Variants = append([]DailyRecapVariant(nil), snapshot.Variants...)
	for index := range snapshot.Variants {
		snapshot.Variants[index].Label = renderBusinessText(snapshot.Variants[index].Label)
		snapshot.Variants[index].MachineBrand = renderBusinessText(snapshot.Variants[index].MachineBrand)
		snapshot.Variants[index].MachineType = renderBusinessText(snapshot.Variants[index].MachineType)
	}
	return snapshot
}

func uppercaseClosingKabupatenSnapshot(snapshot ClosingKabupatenSnapshot) ClosingKabupatenSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.ProvinceName = renderBusinessText(snapshot.ProvinceName)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	snapshot.Rows = append([]ClosingKabupatenRow(nil), snapshot.Rows...)
	for index := range snapshot.Rows {
		snapshot.Rows[index].Location = renderBusinessText(snapshot.Rows[index].Location)
		snapshot.Rows[index].MachineBrand = renderBusinessText(snapshot.Rows[index].MachineBrand)
		snapshot.Rows[index].MachineType = renderBusinessText(snapshot.Rows[index].MachineType)
	}
	return snapshot
}

func uppercaseClosingSnapshot(snapshot ClosingTitikSerahSnapshot) ClosingTitikSerahSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.ProvinceName = renderBusinessText(snapshot.ProvinceName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	snapshot.Rows = append([]ClosingRow(nil), snapshot.Rows...)
	for index := range snapshot.Rows {
		snapshot.Rows[index].MachineBrand = renderBusinessText(snapshot.Rows[index].MachineBrand)
		snapshot.Rows[index].MachineType = renderBusinessText(snapshot.Rows[index].MachineType)
	}
	return snapshot
}
